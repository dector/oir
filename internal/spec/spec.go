// Package spec parses tool specifications such as "gh:dector/ror".
package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// Backend identifies where a tool is fetched from.
type Backend string

// Supported backends.
const (
	BackendGitHub Backend = "github"
)

// Spec describes a tool to install.
type Spec struct {
	Backend Backend
	Owner   string
	Repo    string
	Version string // empty means "latest"
}

// String returns the canonical form, e.g. "github:dector/ror".
func (s Spec) String() string {
	out := string(s.Backend) + ":" + s.Owner + "/" + s.Repo
	if s.Version != "" {
		out += "@" + s.Version
	}
	return out
}

// Key returns the store-relative directory path, e.g. "github/dector/ror".
// The backend names the host, and owner/repo nest underneath it.
func (s Spec) Key() string {
	return strings.ToLower(fmt.Sprintf("%s/%s/%s", s.Backend, s.Owner, s.Repo))
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Parse parses "gh:owner/repo", "github:owner/repo" or "owner/repo".
func Parse(raw string) (Spec, error) {
	rest := strings.TrimSpace(raw)
	if rest == "" {
		return Spec{}, fmt.Errorf("empty tool spec")
	}

	backend := BackendGitHub
	if i := strings.Index(rest, ":"); i >= 0 {
		switch prefix := strings.ToLower(rest[:i]); prefix {
		case "gh", "github":
			backend = BackendGitHub
		default:
			return Spec{}, fmt.Errorf("unknown backend %q (supported: gh)", rest[:i])
		}
		rest = rest[i+1:]
	}

	version := ""
	if i := strings.Index(rest, "@"); i >= 0 {
		version = rest[i+1:]
		rest = rest[:i]
	}

	owner, repo, ok := strings.Cut(rest, "/")
	if !ok || owner == "" || repo == "" {
		return Spec{}, fmt.Errorf("invalid tool spec %q: want owner/repo", raw)
	}
	if !nameRE.MatchString(owner) {
		return Spec{}, fmt.Errorf("invalid owner %q", owner)
	}
	if !nameRE.MatchString(repo) {
		return Spec{}, fmt.Errorf("invalid repo %q", repo)
	}

	return Spec{Backend: backend, Owner: owner, Repo: repo, Version: version}, nil
}
