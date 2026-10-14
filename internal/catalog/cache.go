package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/dector/oir/internal/alias"
)

// Source order determines alias precedence.
type Source struct{ Name, IndexURL string }

func OfficialSources() []Source {
	return []Source{{"official", "https://raw.githubusercontent.com/dector/oir/registry/index.kdl"}}
}

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
type Options struct {
	Sources  []Source
	CacheDir string
	Client   HTTPClient
	Now      func() time.Time
	Warn     func(error)
}
type Catalog struct{ options Options }
type Entry struct{ Key, Spec, Source string }
type snapshot struct {
	Fetched           time.Time
	Manifest, Aliases []byte
}

func DefaultCacheDir() (string, error) {
	if dir := os.Getenv("OIR_CACHE_DIR"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "oir"), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "oir"), nil
}
func New(o Options) (*Catalog, error) {
	if o.Sources == nil {
		o.Sources = OfficialSources()
	}
	if o.CacheDir == "" {
		var err error
		o.CacheDir, err = DefaultCacheDir()
		if err != nil {
			return nil, err
		}
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 30 * time.Second}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Catalog{options: o}, nil
}
func (c *Catalog) path(s Source) string {
	return filepath.Join(c.options.CacheDir, fmt.Sprintf("%x.json", sha256.Sum256([]byte(s.Name+"\x00"+s.IndexURL))))
}
func validate(s snapshot) ([]alias.Entry, error) {
	if s.Fetched.IsZero() {
		return nil, errors.New("missing snapshot timestamp")
	}
	if _, err := ParseManifest(s.Manifest); err != nil {
		return nil, err
	}
	return ParseAliases(s.Aliases)
}
func (c *Catalog) load(s Source) (snapshot, []alias.Entry, error) {
	var snap snapshot
	data, err := os.ReadFile(c.path(s))
	if err != nil {
		return snap, nil, err
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return snap, nil, err
	}
	entries, err := validate(snap)
	return snap, entries, err
}
func (c *Catalog) download(ctx context.Context, address string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.options.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", address, res.StatusCode)
	}
	const maxSize = 8 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("%s: registry file too large", address)
	}
	return data, nil
}
func (c *Catalog) refresh(ctx context.Context, source Source) ([]alias.Entry, error) {
	manifest, err := c.download(ctx, source.IndexURL)
	if err != nil {
		return nil, err
	}
	m, err := ParseManifest(manifest)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(source.IndexURL)
	if err != nil {
		return nil, err
	}
	address := base.ResolveReference(&url.URL{Path: m.File}).String()
	data, err := c.download(ctx, address)
	if err != nil {
		return nil, err
	}
	entries, err := ParseAliases(data)
	if err != nil {
		return nil, err
	}
	snap := snapshot{Fetched: c.options.Now(), Manifest: manifest, Aliases: data}
	encoded, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.options.CacheDir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(c.options.CacheDir, "snapshot-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(encoded); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), c.path(source)); err != nil {
		return nil, err
	}
	return entries, nil
}

// Refresh attempts every source, retaining successful updates even on failure.
func (c *Catalog) Refresh(ctx context.Context) error {
	var failures []error
	for _, s := range c.options.Sources {
		if _, err := c.refresh(ctx, s); err != nil {
			failures = append(failures, fmt.Errorf("registry %s: %w", s.Name, err))
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return fmt.Errorf("refresh failed for %d registries: %w", len(failures), errors.Join(failures...))
}

// Entries uses a usable stale snapshot only when automatic refresh fails.
func (c *Catalog) Entries(ctx context.Context) ([]Entry, error) {
	var result []Entry
	for _, source := range c.options.Sources {
		snap, entries, cacheErr := c.load(source)
		if cacheErr != nil || c.options.Now().Sub(snap.Fetched) > 24*time.Hour {
			fresh, err := c.refresh(ctx, source)
			if err != nil {
				warning := fmt.Errorf("registry %s: %w", source.Name, err)
				if cacheErr != nil {
					return nil, warning
				}
				if c.options.Warn != nil {
					c.options.Warn(warning)
				}
			} else {
				entries = fresh
			}
		}
		for _, e := range entries {
			result = append(result, Entry{e.Key, e.Spec, source.Name})
		}
	}
	return result, nil
}
