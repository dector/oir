// Package link maintains the user-facing symlinks in a bin directory.
package link

import (
	"fmt"
	"os"
	"path/filepath"
)

// Ensure points linkPath at target, creating parent directories as needed.
// It reports whether the link was created or changed.
//
// It refuses to clobber anything oir does not manage:
//
//   - an existing symlink is replaced only when its resolved destination is
//     owned by oir (the owned callback);
//   - a regular file or directory is never replaced.
//
// force overrides both refusals.
func Ensure(target, linkPath string, owned func(string) bool, force bool) (bool, error) {
	skip, err := checkExisting(linkPath, target, owned, force)
	if err != nil {
		return false, err
	}
	if skip {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return false, err
	}

	tmp, err := reserveTempName(linkPath)
	if err != nil {
		return false, err
	}
	if err := os.Symlink(target, tmp); err != nil {
		return false, err
	}

	if err := os.Rename(tmp, linkPath); err != nil {
		os.Remove(tmp)
		return false, err
	}

	return true, nil
}

// checkExisting reports whether the link is already correct, or an error if
// replacing it is unsafe.
func checkExisting(linkPath, target string, owned func(string) bool, force bool) (bool, error) {
	existing, err := os.Lstat(linkPath)
	switch {
	case os.IsNotExist(err):
		return false, nil
	case err != nil:
		return false, err
	}

	if existing.Mode()&os.ModeSymlink == 0 {
		if force {
			return false, nil
		}
		return false, fmt.Errorf("refusing to replace %s: it is a regular file, not an oir-managed symlink (use --force to override)", linkPath)
	}

	current, err := os.Readlink(linkPath)
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(current) {
		current = filepath.Join(filepath.Dir(linkPath), current)
	}
	current = filepath.Clean(current)

	if current == filepath.Clean(target) {
		return true, nil
	}
	if !owned(current) && !force {
		return false, fmt.Errorf("refusing to replace %s: it points outside the oir store (-> %s) (use --force to override)", linkPath, current)
	}

	return false, nil
}

// reserveTempName creates a unique sibling path for an atomic symlink swap.
func reserveTempName(linkPath string) (string, error) {
	dir := filepath.Dir(linkPath)

	f, err := os.CreateTemp(dir, filepath.Base(linkPath)+".tmp-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	f.Close()

	if err := os.Remove(name); err != nil {
		return "", err
	}

	return name, nil
}
