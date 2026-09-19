// Package gh implements the GitHub Releases registry backend.
//
// The backend never chooses where files live. Materialize writes only into the
// folder it is given by the caller, and reports the path of the binary it
// placed so the caller can link it.
package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dector/oir/internal/archive"
	"github.com/dector/oir/internal/progress"
	"github.com/dector/oir/internal/registry"
	"github.com/dector/oir/internal/verify"
	"github.com/dector/oir/internal/version"
)

// DefaultBaseURL is the public GitHub API endpoint.
const DefaultBaseURL = "https://api.github.com"

// Client is the GitHub Releases backend. It implements registry.Backend.
type Client struct {
	BaseURL   string
	Token     string
	UserAgent string
	HTTP      *http.Client
}

// New builds a client, picking up GITHUB_TOKEN or GH_TOKEN if present.
func New() *Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}

	return &Client{
		BaseURL:   DefaultBaseURL,
		Token:     token,
		UserAgent: "oir/" + version.Version,
		HTTP:      &http.Client{},
	}
}

// Resolve returns the release for version, or the latest release when version
// is empty.
//
// Some repositories only publish prereleases, in which case GitHub's
// /releases/latest endpoint returns 404. Resolve then falls back to the list.
func (c *Client) Resolve(ctx context.Context, owner, repo, version string) (*registry.Release, error) {
	if version != "" {
		var rel releaseJSON
		path := fmt.Sprintf("/repos/%s/%s/releases/tags/%s",
			url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(version))
		if err := c.getJSON(ctx, path, &rel); err != nil {
			return nil, err
		}

		return rel.registry(), nil
	}

	var latest releaseJSON
	err := c.getJSON(ctx, fmt.Sprintf("/repos/%s/%s/releases/latest",
		url.PathEscape(owner), url.PathEscape(repo)), &latest)
	if err == nil {
		return latest.registry(), nil
	}

	rels, listErr := c.releases(ctx, owner, repo, 30)
	if listErr != nil {
		return nil, err
	}
	for i := range rels {
		if !rels[i].Draft && !rels[i].Prerelease {
			return rels[i].registry(), nil
		}
	}
	if len(rels) > 0 {
		return rels[0].registry(), nil
	}

	return nil, registry.ErrNoReleases
}

// Materialize implements registry.Backend. It downloads req.Asset, verifies
// its checksum and writes the extracted binary into req.Dir.
func (c *Client) Materialize(ctx context.Context, req registry.MaterializeRequest) (string, error) {
	log := req.Log
	if log == nil {
		log = io.Discard
	}

	if err := os.MkdirAll(req.Dir, 0o755); err != nil {
		return "", err
	}

	download, err := os.CreateTemp(req.Dir, ".oir-download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(download.Name())

	if err := c.download(ctx, log, req.Asset, download); err != nil {
		download.Close()
		return "", err
	}
	if err := download.Close(); err != nil {
		return "", err
	}

	if err := c.verify(ctx, log, req, download.Name()); err != nil {
		return "", err
	}

	return placeBinary(download.Name(), req.Repo, req.Dir)
}

// download streams an asset into dst, reporting progress to log.
func (c *Client) download(ctx context.Context, log io.Writer, asset registry.Asset, dst *os.File) error {
	rc, err := c.open(ctx, asset)
	if err != nil {
		return err
	}
	defer rc.Close()

	prog := progress.New(log, "downloading "+asset.Name, asset.Size)
	if _, err := io.Copy(dst, io.TeeReader(rc, prog)); err != nil {
		return err
	}
	prog.Done()

	return nil
}

// verify checks the downloaded file against the asset digest, a published
// checksums file, or the --no-verify escape hatch.
func (c *Client) verify(ctx context.Context, log io.Writer, req registry.MaterializeRequest, path string) error {
	if req.Asset.Digest != "" {
		if err := verify.CheckSHA256(path, req.Asset.Digest); err != nil {
			return err
		}
		fmt.Fprintf(log, "  verified %s\n", req.Asset.Digest)

		return nil
	}

	if sums, ok := findChecksumAsset(req.Release.Assets); ok {
		digest, err := c.fetchChecksum(ctx, sums, req.Asset.Name)
		if err != nil {
			return err
		}
		if err := verify.CheckSHA256(path, digest); err != nil {
			return err
		}
		fmt.Fprintf(log, "  verified sha256:%s (from %s)\n", digest, sums.Name)

		return nil
	}

	if req.NoVerify {
		fmt.Fprintf(log, "warning: no checksum published for %s, skipping verification\n", req.Asset.Name)
		return nil
	}

	return fmt.Errorf("no checksum published for %s; rerun with --no-verify to install anyway", req.Asset.Name)
}

func (c *Client) fetchChecksum(ctx context.Context, sums registry.Asset, filename string) (string, error) {
	rc, err := c.open(ctx, sums)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	digest, err := verify.FindChecksum(io.LimitReader(rc, 4<<20), filename)
	if err != nil {
		return "", fmt.Errorf("%s: %w", sums.Name, err)
	}

	return digest, nil
}

// open starts downloading an asset and returns the raw response body.
func (c *Client) open(ctx context.Context, a registry.Asset) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, "application/octet-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	return resp.Body, nil
}

// placeBinary writes the downloaded artifact into destDir/<repo> and returns
// the final path. Archives are unpacked and searched for the tool binary; raw
// single-file binaries are copied as-is.
func placeBinary(downloadPath, repo, destDir string) (string, error) {
	src := downloadPath

	isArchive, err := archive.IsArchive(downloadPath)
	if err != nil {
		return "", err
	}
	if isArchive {
		tmpDir, err := os.MkdirTemp(destDir, ".oir-extract-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmpDir)

		if err := archive.Extract(downloadPath, tmpDir); err != nil {
			return "", err
		}

		src, err = archive.FindBinary(tmpDir, repo)
		if err != nil {
			return "", err
		}
	}

	return copyBinary(src, repo, destDir)
}

// copyBinary copies the tool binary at src to destDir/<repo>, making it
// executable. It returns the final path.
func copyBinary(src, repo, destDir string) (string, error) {
	tmp, err := os.CreateTemp(destDir, ".oir-binary-*")
	if err != nil {
		return "", err
	}
	defer func() {
		if tmp != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	if _, err := io.Copy(tmp, in); err != nil {
		return "", err
	}
	if err := tmp.Chmod(0o755); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	final := filepath.Join(destDir, repo)
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", err
	}
	tmp = nil

	return final, nil
}

// releases lists releases, newest first.
func (c *Client) releases(ctx context.Context, owner, repo string, perPage int) ([]releaseJSON, error) {
	if perPage <= 0 {
		perPage = 30
	}

	var rels []releaseJSON
	path := fmt.Sprintf("/repos/%s/%s/releases?per_page=%d",
		url.PathEscape(owner), url.PathEscape(repo), perPage)
	if err := c.getJSON(ctx, path, &rels); err != nil {
		return nil, err
	}

	return rels, nil
}

// releaseJSON is the subset of a GitHub release oir cares about.
type releaseJSON struct {
	TagName    string      `json:"tag_name"`
	Draft      bool        `json:"draft"`
	Prerelease bool        `json:"prerelease"`
	Assets     []assetJSON `json:"assets"`
}

// assetJSON is the subset of a GitHub release asset oir cares about.
type assetJSON struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	Size               int64     `json:"size"`
	Digest             string    `json:"digest"`
	BrowserDownloadURL string    `json:"browser_download_url"`
	CreatedAt          time.Time `json:"created_at"`
}

func (r releaseJSON) registry() *registry.Release {
	assets := make([]registry.Asset, len(r.Assets))
	for i, a := range r.Assets {
		assets[i] = registry.Asset{
			ID:        a.ID,
			Name:      a.Name,
			Size:      a.Size,
			Digest:    a.Digest,
			URL:       a.BrowserDownloadURL,
			CreatedAt: a.CreatedAt,
		}
	}

	return &registry.Release{Tag: r.TagName, Assets: assets}
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req, "application/vnd.github+json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.apiError(resp)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) setHeaders(req *http.Request, accept string) {
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found: %s", trimBody(body))
	}
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		reset := resp.Header.Get("X-RateLimit-Reset")
		hint := "set GITHUB_TOKEN to raise the limit"
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			hint = fmt.Sprintf("rate limit resets at %s; %s",
				time.Unix(ts, 0).Format(time.RFC3339), hint)
		}
		return fmt.Errorf("GitHub API rate limit exceeded: %s", hint)
	}

	msg := trimBody(body)
	var apiErr struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
		msg = apiErr.Message
	}

	return fmt.Errorf("GitHub API error: %s: %s", resp.Status, msg)
}

func trimBody(b []byte) string {
	if len(b) > 300 {
		b = b[:300]
	}

	return string(b)
}

// findChecksumAsset locates a published checksums file among the release assets.
func findChecksumAsset(assets []registry.Asset) (registry.Asset, bool) {
	for _, a := range assets {
		n := strings.ToLower(a.Name)
		switch {
		case strings.Contains(n, "checksum"),
			strings.Contains(n, "sha256sum"),
			strings.HasSuffix(n, ".sha256"),
			n == "sha256sums.txt":
			return a, true
		}
	}

	return registry.Asset{}, false
}
