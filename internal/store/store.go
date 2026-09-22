// Package store manages the on-disk install directory.
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store holds installed tools under a single root directory.
type Store struct {
	Root string
}

// Default returns the store rooted at the default directory.
func Default() (*Store, error) {
	root, err := DefaultDir()
	if err != nil {
		return nil, err
	}

	return &Store{Root: root}, nil
}

// DefaultDir returns the default store root: $OIR_DATA_DIR,
// $XDG_DATA_HOME/oir or ~/.local/share/oir.
func DefaultDir() (string, error) {
	if dir := os.Getenv("OIR_DATA_DIR"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "oir"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".local", "share", "oir"), nil
}

// Dir returns the install directory for a tool key and version.
func (s *Store) Dir(key, version string) string {
	return filepath.Join(s.toolDir(key), SanitizeVersion(version))
}

// toolDir is the directory holding every version of a tool.
func (s *Store) toolDir(key string) string {
	return filepath.Join(s.Root, "installs", key)
}

// BinaryPath returns the expected path of an installed binary.
func (s *Store) BinaryPath(key, version, name string) string {
	return filepath.Join(s.Dir(key, version), name)
}

// Installed describes one installed version of a tool.
type Installed struct {
	Key     string // store key, e.g. "github/dector/ror"
	Version string // version directory, e.g. "v1.2.3"
	Meta    Meta   // metadata recorded at install time, zero when absent
}

// List returns every installed version, sorted by key then version.
//
// A version directory is one that holds the installed files directly, i.e. it
// contains regular files but no subdirectories. This matches the layout written
// by Place and also finds installs made before metadata was recorded.
func (s *Store) List() ([]Installed, error) {
	root := filepath.Join(s.Root, "installs")

	var out []Installed
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		// Skip staging and trash directories left by Place, and any other
		// hidden directory.
		if path != root && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}

		isVersion, hasFile, err := versionDir(path)
		if err != nil {
			return err
		}
		if path == root || !isVersion || !hasFile {
			return nil
		}

		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)

		meta, _, err := s.ReadMeta(key, d.Name())
		if err != nil {
			return err
		}

		out = append(out, Installed{Key: key, Version: d.Name(), Meta: meta})

		return fs.SkipDir
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Version < out[j].Version
	})

	return out, nil
}

// versionDir reports whether path directly holds an installed version: it
// contains the metadata file written by WriteMeta, or, for installs made
// before metadata was recorded, it holds regular files but no subdirectories.
func versionDir(path string) (isVersion, hasFile bool, err error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, false, err
	}

	isVersion = true
	for _, e := range entries {
		if e.Name() == metaFile {
			// A package install keeps subdirectories next to the binary, so the
			// metadata file is the reliable marker.
			return true, true, nil
		}
		if e.IsDir() {
			isVersion = false
		} else if !strings.HasPrefix(e.Name(), ".") {
			hasFile = true
		}
	}

	return isVersion, hasFile, nil
}

// Has reports whether a version directory exists and is non-empty.
func (s *Store) Has(key, version string) bool {
	entries, err := os.ReadDir(s.Dir(key, version))

	return err == nil && len(entries) > 0
}

// Owns reports whether path lives inside the store root.
func (s *Store) Owns(path string) bool {
	rel, err := filepath.Rel(s.Root, filepath.Clean(path))
	if err != nil {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Staging creates a temporary directory next to a tool's version directories.
//
// It lives on the same filesystem as the final install, so Adopt can rename it
// into place atomically. The caller is expected to remove it when not adopted.
func (s *Store) Staging(key string) (string, error) {
	parent := s.toolDir(key)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}

	return os.MkdirTemp(parent, ".staging-*")
}

// Adopt atomically moves a staged install into its final version directory,
// replacing any previous version. On failure the previous version is restored.
func (s *Store) Adopt(key, version, stagedDir string) error {
	finalDir := s.Dir(key, version)
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		return err
	}

	trash := ""
	if _, err := os.Lstat(finalDir); err == nil {
		trash = finalDir + ".old"
		_ = os.RemoveAll(trash)
		if err := os.Rename(finalDir, trash); err != nil {
			return err
		}
	}

	if err := os.Rename(stagedDir, finalDir); err != nil {
		if trash != "" {
			_ = os.Rename(trash, finalDir) // best-effort restore
		}

		return err
	}
	if trash != "" {
		_ = os.RemoveAll(trash)
	}

	return nil
}

// Remove deletes an installed version.
func (s *Store) Remove(key, version string) error {
	return os.RemoveAll(s.Dir(key, version))
}

// ClearExcept removes everything in a version directory except the given
// slash-separated path and the metadata file. Directories on the way to keep
// are retained, so a nested binary survives. It is used to drop package files
// when a tool switches to binary-only mode.
func (s *Store) ClearExcept(key, version, keep string) error {
	keep = filepath.Clean(filepath.FromSlash(keep))
	if keep == "." || keep == ".." {
		return fmt.Errorf("invalid path to keep: %q", keep)
	}

	return clearExcept(s.Dir(key, version), keep)
}

func clearExcept(dir, keep string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	head, rest, hasRest := strings.Cut(keep, string(filepath.Separator))
	for _, e := range entries {
		if e.Name() == metaFile {
			continue
		}
		if e.Name() != head {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
			continue
		}
		if !hasRest {
			continue
		}
		if e.IsDir() {
			if err := clearExcept(filepath.Join(dir, e.Name()), rest); err != nil {
				return err
			}
		}
	}

	return nil
}

// SanitizeVersion makes a git tag safe to use as a directory name.
func SanitizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "latest"
	}

	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-")
	v = replacer.Replace(v)

	return strings.Trim(v, ".")
}

// ErrNotOwned is returned when a path is outside the store.
var ErrNotOwned = fmt.Errorf("path is not owned by the oir store")
