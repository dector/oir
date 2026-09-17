// Package store manages the on-disk install directory.
package store

import (
	"errors"
	"fmt"
	"io"
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

// Default returns the store rooted at $OIR_DATA_DIR,
// $XDG_DATA_HOME/oir or ~/.local/share/oir.
func Default() (*Store, error) {
	if dir := os.Getenv("OIR_DATA_DIR"); dir != "" {
		return &Store{Root: dir}, nil
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return &Store{Root: filepath.Join(dir, "oir")}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return &Store{Root: filepath.Join(home, ".local", "share", "oir")}, nil
}

// Dir returns the install directory for a tool key and version.
func (s *Store) Dir(key, version string) string {
	return filepath.Join(s.Root, "installs", key, SanitizeVersion(version))
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

// versionDir reports whether path directly holds installed files: no
// subdirectories, and at least one non-hidden regular file.
func versionDir(path string) (isVersion, hasFile bool, err error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, false, err
	}

	isVersion = true
	for _, e := range entries {
		if e.IsDir() {
			return false, false, nil
		}
		if !strings.HasPrefix(e.Name(), ".") {
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

// Place copies the binary at src into the store at installs/<key>/<version>/<name>.
//
// The swap is atomic: the new binary is staged in a sibling temp directory and
// renamed into place, so an interrupted install never leaves a partial binary.
// The returned path is the final binary location.
func (s *Store) Place(key, version, src, name string) (string, error) {
	finalDir := s.Dir(key, version)
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		return "", err
	}

	tmpDir, err := os.MkdirTemp(filepath.Dir(finalDir), ".staging-")
	if err != nil {
		return "", err
	}
	defer func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	}()

	staged := filepath.Join(tmpDir, name)
	if err := copyFile(src, staged, 0o755); err != nil {
		return "", err
	}

	// Move any existing install aside, then swap the staged copy in.
	if _, err := os.Lstat(finalDir); err == nil {
		trash := finalDir + ".old"
		_ = os.RemoveAll(trash)
		if err := os.Rename(finalDir, trash); err != nil {
			return "", err
		}
		defer os.RemoveAll(trash)
	}

	if err := os.Rename(tmpDir, finalDir); err != nil {
		return "", err
	}
	// The staging dir has been renamed away; stop the deferred cleanup from
	// trying to remove the live install.
	tmpDir = ""

	return filepath.Join(finalDir, name), nil
}

// Remove deletes an installed version.
func (s *Store) Remove(key, version string) error {
	return os.RemoveAll(s.Dir(key, version))
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

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	return os.Chmod(dst, perm)
}

// ErrNotOwned is returned when a path is outside the store.
var ErrNotOwned = fmt.Errorf("path is not owned by the oir store")
