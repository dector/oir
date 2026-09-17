// Package registry defines the abstraction over remote tool registries.
//
// A Backend knows how to resolve releases and materialize their artifacts for
// one kind of registry, for example GitHub Releases. Backends write files only
// into a folder the caller assigns: they never choose where installs live.
package registry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"
)

// Platform is the target os/arch pair used to match release assets.
type Platform struct {
	OS   string // runtime.GOOS values: linux, darwin, windows, ...
	Arch string // runtime.GOARCH values: amd64, arm64, ...
}

// CurrentPlatform reports the platform oir is running on.
func CurrentPlatform() Platform {
	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// Asset is a single downloadable file attached to a release.
type Asset struct {
	ID        int64
	Name      string
	Size      int64
	Digest    string // e.g. "sha256:<hex>", when the registry publishes one
	URL       string
	CreatedAt time.Time
}

// Release is a published version of a tool.
type Release struct {
	Tag    string
	Assets []Asset
}

// MaterializeRequest describes one artifact to install into an assigned folder.
type MaterializeRequest struct {
	Release  *Release  // release the asset belongs to, used for checksums
	Asset    Asset     // asset to download
	Dir      string    // staging folder assigned by the caller
	Repo     string    // repository name, used to locate and name the binary
	NoVerify bool      // skip checksum verification
	Log      io.Writer // optional status and progress output
}

// Backend resolves and installs tools from one kind of remote registry.
type Backend interface {
	// Resolve returns the release for version, or the latest release when
	// version is empty.
	Resolve(ctx context.Context, owner, repo, version string) (*Release, error)

	// Materialize downloads req.Asset, verifies it and writes the installed
	// binary into req.Dir. It returns the absolute path of the placed binary.
	//
	// req.Dir is a staging folder the caller assigns and will adopt
	// atomically. The backend must not choose it or derive it from global
	// state: it only ever writes into the folder it is given.
	Materialize(ctx context.Context, req MaterializeRequest) (string, error)
}

// Backends maps backend names as used in a spec to their implementations.
type Backends map[string]Backend

// Get returns the backend registered under name.
func (b Backends) Get(name string) (Backend, error) {
	backend, ok := b[name]
	if !ok {
		return nil, fmt.Errorf("unknown registry backend %q", name)
	}

	return backend, nil
}

// ErrNoReleases is returned when a repository has no usable releases.
var ErrNoReleases = errors.New("no releases found")
