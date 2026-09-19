// Package install orchestrates resolving, downloading and installing a tool
// from a remote registry.
package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dector/oir/internal/link"
	"github.com/dector/oir/internal/registry"
	"github.com/dector/oir/internal/spec"
	"github.com/dector/oir/internal/store"
	"github.com/dector/oir/internal/style"
)

// Options configures an Installer.
type Options struct {
	// Backends maps a spec's backend name to its registry implementation.
	Backends registry.Backends
	// StoreDir is the folder oir keeps installed tools in. The installer
	// never chooses it on its own.
	StoreDir string
	// BinDir is where the user-facing symlinks are created.
	BinDir string
	// Platform is the target platform used to pick release assets.
	Platform registry.Platform
	// NoVerify skips checksum verification (unsafe).
	NoVerify bool
	// Force replaces links oir already owns.
	Force  bool
	Stdout io.Writer
	Stderr io.Writer
}

// Installer performs installs with a fixed configuration.
type Installer struct {
	Backends registry.Backends
	Store    *store.Store
	BinDir   string
	Platform registry.Platform
	NoVerify bool
	Force    bool
	Stdout   io.Writer
	Stderr   io.Writer
}

// New builds an Installer from options.
func New(opts Options) *Installer {
	stdout, stderr := opts.Stdout, opts.Stderr
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	return &Installer{
		Backends: opts.Backends,
		Store:    &store.Store{Root: opts.StoreDir},
		BinDir:   opts.BinDir,
		Platform: opts.Platform,
		NoVerify: opts.NoVerify,
		Force:    opts.Force,
		Stdout:   stdout,
		Stderr:   stderr,
	}
}

// Result describes a finished install.
type Result struct {
	Spec     spec.Spec
	Version  string
	Asset    string
	Binary   string
	Link     string
	Changed  bool
	UpToDate bool // the requested version was already installed unchanged
}

// Run resolves and installs sp.
//
// The backend named by sp downloads the artifact into a staging folder the
// installer assigns. The installer then adopts that folder atomically into the
// store, records metadata, and links the binary into BinDir.
func (in *Installer) Run(ctx context.Context, sp spec.Spec) (*Result, error) {
	backend, err := in.Backends.Get(string(sp.Backend))
	if err != nil {
		return nil, err
	}

	rel, err := backend.Resolve(ctx, sp.Owner, sp.Repo, sp.Version)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", sp, err)
	}

	version := rel.Tag
	key := sp.Key()
	name := sp.Repo
	binaryPath := in.Store.BinaryPath(key, version, name)

	res := &Result{Spec: sp, Version: version, Binary: binaryPath, Link: filepath.Join(in.BinDir, name)}

	asset, err := registry.Pick(rel.Assets, sp.Repo, in.Platform)
	if err != nil {
		return nil, err
	}
	res.Asset = asset.Name

	if in.Store.Has(key, version) && !in.Force {
		meta, ok, _ := in.Store.ReadMeta(key, version)
		if ok && !assetChanged(meta, asset) {
			fmt.Fprintf(in.Stdout, "%s %s is already installed\n", style.Name(sp.String()), version)
			changed, err := in.linkBinary(name, res)
			if err != nil {
				return nil, err
			}
			res.Changed = changed
			res.UpToDate = true

			return res, nil
		}
		if ok {
			fmt.Fprintf(in.Stderr, "%s %s %s changed (%s -> %s), reinstalling\n",
				style.Warn("warn:"), style.Name(sp.String()), version, meta.Asset, asset.Name)
		}
		// No metadata, or the asset changed: reinstall to record (or refresh)
		// the install. This also backfills metadata for pre-existing installs.
	}

	staging, err := in.Store.Staging(key)
	if err != nil {
		return nil, err
	}
	defer func() {
		if staging != "" {
			os.RemoveAll(staging)
		}
	}()

	binPath, err := backend.Materialize(ctx, registry.MaterializeRequest{
		Release:  rel,
		Asset:    asset,
		Dir:      staging,
		Repo:     name,
		NoVerify: in.NoVerify,
		Log:      in.Stderr,
	})
	if err != nil {
		return nil, err
	}

	if err := in.Store.Adopt(key, version, staging); err != nil {
		return nil, err
	}
	staging = "" // ownership has moved into the store

	res.Binary = filepath.Join(in.Store.Dir(key, version), filepath.Base(binPath))

	if err := in.Store.WriteMeta(key, version, store.Meta{
		Asset:   asset.Name,
		Digest:  asset.Digest,
		AssetID: asset.ID,
	}); err != nil {
		fmt.Fprintf(in.Stderr, "%s record install metadata: %v\n", style.Warn("warning:"), err)
	}

	changed, err := in.linkBinary(name, res)
	if err != nil {
		return nil, err
	}
	res.Changed = changed

	return res, nil
}

// linkBinary points BinDir/<name> at the installed binary.
func (in *Installer) linkBinary(name string, res *Result) (bool, error) {
	linkPath := filepath.Join(in.BinDir, name)

	changed, err := link.Ensure(res.Binary, linkPath, in.Store.Owns, in.Force)
	if err != nil {
		return false, err
	}
	res.Link = linkPath

	return changed, nil
}

// assetChanged reports whether the release asset differs from the one recorded
// at install time. It prefers the asset digest, then the asset id, and falls
// back to the asset name.
func assetChanged(m store.Meta, a registry.Asset) bool {
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
