// Package install orchestrates resolving, downloading and installing a tool
// from a GitHub release.
package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dector/oir/internal/archive"
	"github.com/dector/oir/internal/gh"
	"github.com/dector/oir/internal/link"
	"github.com/dector/oir/internal/spec"
	"github.com/dector/oir/internal/store"
	"github.com/dector/oir/internal/verify"
)

// Installer performs installs with a fixed configuration.
type Installer struct {
	Client   *gh.Client
	Store    *store.Store
	BinDir   string
	Platform gh.Platform
	NoVerify bool
	Force    bool
	Stdout   io.Writer
	Stderr   io.Writer
}

// Result describes a finished install.
type Result struct {
	Spec    spec.Spec
	Version string
	Asset   string
	Binary  string
	Link    string
	Changed bool
}

// Run resolves and installs sp.
//
// Only the implicit "latest" version is supported for now: the resolved tag
// becomes the version directory, and the binary is symlinked into BinDir.
func (in *Installer) Run(ctx context.Context, sp spec.Spec) (*Result, error) {
	rel, err := in.resolve(ctx, sp)
	if err != nil {
		return nil, err
	}

	version := rel.TagName
	key := sp.Key()
	name := sp.Repo
	binaryPath := in.Store.BinaryPath(key, version, name)

	res := &Result{Spec: sp, Version: version, Binary: binaryPath, Link: filepath.Join(in.BinDir, name)}

	asset, err := gh.PickAsset(rel.Assets, sp.Repo, in.Platform)
	if err != nil {
		return nil, err
	}
	res.Asset = asset.Name

	if in.Store.Has(key, version) && !in.Force {
		meta, ok, _ := in.Store.ReadMeta(key, version)
		if ok && !assetChanged(meta, asset) {
			fmt.Fprintf(in.Stdout, "%s %s is already installed\n", sp, version)
			changed, err := in.linkBinary(key, version, name, res)
			if err != nil {
				return nil, err
			}
			res.Changed = changed

			return res, nil
		}
		if ok {
			fmt.Fprintf(in.Stderr, "warn: %s %s changed (%s -> %s), reinstalling\n",
				sp, version, meta.Asset, asset.Name)
		}
		// No metadata, or the asset changed: reinstall to record (or refresh)
		// the install. This also backfills metadata for pre-existing installs.
	}

	tmpFile, err := in.download(ctx, asset)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)

	if err := in.verifyDownload(ctx, rel, asset, tmpFile); err != nil {
		return nil, err
	}

	binPath, err := in.extractBinary(tmpFile, sp.Repo)
	if err != nil {
		return nil, err
	}

	if _, err := in.Store.Place(key, version, binPath, name); err != nil {
		return nil, fmt.Errorf("install into store: %w", err)
	}

	if err := in.Store.WriteMeta(key, version, store.Meta{
		Asset:   asset.Name,
		Digest:  asset.Digest,
		AssetID: asset.ID,
	}); err != nil {
		fmt.Fprintf(in.Stderr, "warning: record install metadata: %v\n", err)
	}

	changed, err := in.linkBinary(key, version, name, res)
	if err != nil {
		return nil, err
	}
	res.Changed = changed

	return res, nil
}

// linkBinary points BinDir/<name> at the installed binary.
func (in *Installer) linkBinary(key, version, name string, res *Result) (bool, error) {
	binaryPath := in.Store.BinaryPath(key, version, name)
	linkPath := filepath.Join(in.BinDir, name)

	changed, err := link.Ensure(binaryPath, linkPath, in.Store.Owns, in.Force)
	if err != nil {
		return false, err
	}
	res.Link = linkPath

	return changed, nil
}

func (in *Installer) resolve(ctx context.Context, sp spec.Spec) (*gh.Release, error) {
	if sp.Version != "" {
		rel, err := in.Client.ReleaseByTag(ctx, sp.Owner, sp.Repo, sp.Version)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", sp, err)
		}
		return rel, nil
	}

	rel, err := in.Client.LatestRelease(ctx, sp.Owner, sp.Repo)
	if err == nil {
		return rel, nil
	}

	// Some repositories only publish prereleases, in which case GitHub's
	// /releases/latest endpoint returns 404. Fall back to the list.
	rels, listErr := in.Client.Releases(ctx, sp.Owner, sp.Repo, 30)
	if listErr != nil {
		return nil, fmt.Errorf("resolve %s: %w", sp, err)
	}
	for i := range rels {
		if !rels[i].Draft && !rels[i].Prerelease {
			return &rels[i], nil
		}
	}
	if len(rels) > 0 {
		return &rels[0], nil
	}

	return nil, fmt.Errorf("resolve %s: %w", sp, gh.ErrNoReleases)
}

func (in *Installer) download(ctx context.Context, asset gh.Asset) (string, error) {
	resp, err := in.Client.OpenAsset(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp("", "oir-download-*")
	if err != nil {
		return "", err
	}

	prog := newProgress(in.Stderr, "downloading "+asset.Name, asset.Size)
	if _, err := io.Copy(tmp, io.TeeReader(resp.Body, prog)); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	prog.Done()

	return tmp.Name(), nil
}

func (in *Installer) verifyDownload(ctx context.Context, rel *gh.Release, asset gh.Asset, path string) error {
	if asset.Digest != "" {
		if err := verify.CheckSHA256(path, asset.Digest); err != nil {
			return err
		}
		fmt.Fprintf(in.Stdout, "  verified %s\n", asset.Digest)

		return nil
	}

	if sums, ok := findChecksumAsset(rel.Assets); ok {
		digest, err := in.fetchChecksum(ctx, sums, asset.Name)
		if err != nil {
			return err
		}
		if err := verify.CheckSHA256(path, digest); err != nil {
			return err
		}
		fmt.Fprintf(in.Stdout, "  verified sha256:%s (from %s)\n", digest, sums.Name)

		return nil
	}

	if in.NoVerify {
		fmt.Fprintf(in.Stderr, "warning: no checksum published for %s, skipping verification\n", asset.Name)
		return nil
	}

	return fmt.Errorf("no checksum published for %s; rerun with --no-verify to install anyway", asset.Name)
}

func (in *Installer) fetchChecksum(ctx context.Context, sums gh.Asset, filename string) (string, error) {
	resp, err := in.Client.OpenAsset(ctx, sums.BrowserDownloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	digest, err := verify.FindChecksum(io.LimitReader(resp.Body, 4<<20), filename)
	if err != nil {
		return "", fmt.Errorf("%s: %w", sums.Name, err)
	}

	return digest, nil
}

func (in *Installer) extractBinary(archivePath, repo string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "oir-extract-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	if err := archive.Extract(archivePath, tmpDir); err != nil {
		return "", err
	}

	bin, err := archive.FindBinary(tmpDir, repo)
	if err != nil {
		return "", err
	}

	// Copy out of the temp tree so the deferred cleanup cannot remove it.
	staged, err := os.CreateTemp("", "oir-binary-*")
	if err != nil {
		return "", err
	}
	defer staged.Close()

	src, err := os.Open(bin)
	if err != nil {
		return "", err
	}
	defer src.Close()

	if _, err := io.Copy(staged, src); err != nil {
		return "", err
	}
	if err := staged.Chmod(0o755); err != nil {
		return "", err
	}

	return staged.Name(), nil
}

// assetChanged reports whether the release asset differs from the one recorded
// at install time. It prefers the GitHub asset digest, then the asset id, and
// falls back to the asset name.
func assetChanged(m store.Meta, a gh.Asset) bool {
	if m.Asset == "" {
		return true
	}
	if m.Digest != "" && a.Digest != "" && m.Digest != a.Digest {
		return true
	}
	if m.AssetID != 0 && a.ID != 0 && m.AssetID != a.ID {
		return true
	}

	return m.Asset != a.Name
}

// findChecksumAsset locates a published checksums file among the release assets.
func findChecksumAsset(assets []gh.Asset) (gh.Asset, bool) {
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

	return gh.Asset{}, false
}
