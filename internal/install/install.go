// Package install orchestrates resolving, downloading and installing a tool
// from a remote registry.
package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	// Full keeps the whole release archive instead of only the binary.
	Full bool
	// OnlyBinary switches an existing package install back to binary-only mode.
	OnlyBinary bool
	// Clear removes the package files when switching to binary-only mode.
	Clear bool
	// Force replaces links oir already owns.
	Force  bool
	Stdout io.Writer
	Stderr io.Writer
}

// Installer performs installs with a fixed configuration.
type Installer struct {
	Backends   registry.Backends
	Store      *store.Store
	BinDir     string
	Platform   registry.Platform
	NoVerify   bool
	Full       bool
	OnlyBinary bool
	Clear      bool
	Force      bool
	Stdout     io.Writer
	Stderr     io.Writer
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
		Backends:   opts.Backends,
		Store:      &store.Store{Root: opts.StoreDir},
		BinDir:     opts.BinDir,
		Platform:   opts.Platform,
		NoVerify:   opts.NoVerify,
		Full:       opts.Full,
		OnlyBinary: opts.OnlyBinary,
		Clear:      opts.Clear,
		Force:      opts.Force,
		Stdout:     stdout,
		Stderr:     stderr,
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
	if in.Full && in.OnlyBinary {
		return nil, fmt.Errorf("--full and --only-binary are mutually exclusive")
	}

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
	dir := in.Store.Dir(key, version)

	res := &Result{
		Spec:    sp,
		Version: version,
		Binary:  in.Store.BinaryPath(key, version, name),
		Link:    filepath.Join(in.BinDir, name),
	}

	asset, err := registry.Pick(rel.Assets, sp.Repo, in.Platform)
	if err != nil {
		return nil, err
	}
	res.Asset = asset.Name

	// A corrupt metadata file reads as absent, so the install self-heals by
	// reinstalling and overwriting it.
	meta, hasMeta, _ := in.Store.ReadMeta(key, version)

	// --full is sticky: once a tool was recorded as a full package, later
	// installs and `update` keep the package layout without the flag.
	// --only-binary clears that choice and takes precedence.
	full := hasMeta && meta.Full
	switch {
	case in.OnlyBinary:
		full = false
	case in.Full:
		full = true
	}

	// Switching an existing install back to binary-only must not touch the
	// files unless --clear is given, so it skips the reinstall path entirely.
	if in.OnlyBinary && in.Store.Has(key, version) {
		return in.switchToBinary(key, version, name, meta, hasMeta, res)
	}

	if in.Store.Has(key, version) && !in.Force {
		if hasMeta && !assetChanged(meta, asset) && meta.Binary != "" && meta.Full == full {
			fmt.Fprintf(in.Stdout, "%s %s %s, already installed\n", style.Muted("skipping"), style.Name(sp.String()), version)
			res.Binary = filepath.Join(dir, filepath.FromSlash(meta.Binary))
			changed, err := in.linkBinary(name, res)
			if err != nil {
				return nil, err
			}
			res.Changed = changed
			res.UpToDate = true

			return res, nil
		}
		if hasMeta {
			switch {
			case assetChanged(meta, asset):
				fmt.Fprintf(in.Stderr, "%s %s %s changed (%s -> %s), reinstalling\n",
					style.Warn("warn:"), style.Name(sp.String()), version, meta.Asset, asset.Name)
			case meta.Binary == "":
				fmt.Fprintf(in.Stderr, "%s %s %s installed by an older oir, reinstalling\n",
					style.Warn("warn:"), style.Name(sp.String()), version)
			default:
				fmt.Fprintf(in.Stderr, "%s %s %s switching to the full package layout, reinstalling\n",
					style.Warn("warn:"), style.Name(sp.String()), version)
			}
		}
		// No metadata, or the layout changed: reinstall to record (or refresh)
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
		Full:     full,
		NoVerify: in.NoVerify,
		Log:      in.Stderr,
	})
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(staging, binPath)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("binary %s is outside the install dir", binPath)
	}
	relPath = filepath.ToSlash(relPath)

	if err := in.Store.Adopt(key, version, staging); err != nil {
		return nil, err
	}
	staging = "" // ownership has moved into the store

	res.Binary = filepath.Join(dir, filepath.FromSlash(relPath))

	if err := in.Store.WriteMeta(key, version, store.Meta{
		Asset:   asset.Name,
		Digest:  asset.Digest,
		AssetID: asset.ID,
		Binary:  relPath,
		Full:    full,
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

// switchToBinary records binary-only mode for an existing install. The package
// files are left untouched unless clear is set, in which case everything except
// the binary and its metadata is removed. It never downloads anything.
func (in *Installer) switchToBinary(key, version, name string, meta store.Meta, hasMeta bool, res *Result) (*Result, error) {
	dir := in.Store.Dir(key, version)

	binRel := name
	if hasMeta && meta.Binary != "" {
		binRel = meta.Binary
	}
	binPath := filepath.Join(dir, filepath.FromSlash(binRel))

	if in.Clear {
		if err := in.Store.ClearExcept(key, version, binRel); err != nil {
			return nil, err
		}
		fmt.Fprintf(in.Stdout, "%s %s %s, package files removed\n",
			style.Muted("cleared"), style.Name(res.Spec.String()), version)
	} else {
		fmt.Fprintf(in.Stdout, "%s %s %s, switched to binary mode\n",
			style.Muted("kept"), style.Name(res.Spec.String()), version)
	}

	meta.Binary = binRel
	meta.Full = false
	if err := in.Store.WriteMeta(key, version, meta); err != nil {
		fmt.Fprintf(in.Stderr, "%s record install metadata: %v\n", style.Warn("warning:"), err)
	}

	res.Binary = binPath
	changed, err := in.linkBinary(name, res)
	if err != nil {
		return nil, err
	}
	res.Changed = changed
	res.UpToDate = true

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
