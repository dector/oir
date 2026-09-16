package link

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) (root string, owned func(string) bool) {
	t.Helper()

	root = t.TempDir()
	s := &StoreRoot{Root: root}

	return root, s.Owns
}

// StoreRoot is a tiny helper so the link tests do not depend on store.
type StoreRoot struct{ Root string }

func (s *StoreRoot) Owns(p string) bool {
	rel, err := filepath.Rel(s.Root, filepath.Clean(p))

	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func TestEnsureCreatesLink(t *testing.T) {
	root, owned := setup(t)
	target := filepath.Join(root, "installs", "ror", "latest", "ror")
	linkPath := filepath.Join(t.TempDir(), "bin", "ror")

	changed, err := Ensure(target, linkPath, owned, false)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !changed {
		t.Error("changed = false, want true")
	}

	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Errorf("link = %q, want %q", got, target)
	}
}

func TestEnsureIsIdempotent(t *testing.T) {
	root, owned := setup(t)
	target := filepath.Join(root, "installs", "ror", "latest", "ror")
	linkPath := filepath.Join(t.TempDir(), "bin", "ror")

	if _, err := Ensure(target, linkPath, owned, false); err != nil {
		t.Fatal(err)
	}

	changed, err := Ensure(target, linkPath, owned, false)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if changed {
		t.Error("changed = true on second call, want false")
	}
}

func TestEnsureRefusesForeignLink(t *testing.T) {
	root, owned := setup(t)
	binDir := t.TempDir()
	linkPath := filepath.Join(binDir, "ror")

	if err := os.Symlink("/etc/hosts", linkPath); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "installs", "ror", "latest", "ror")
	if _, err := Ensure(target, linkPath, owned, false); err == nil {
		t.Fatal("expected refusal for a foreign symlink")
	}

	got, _ := os.Readlink(linkPath)
	if got != "/etc/hosts" {
		t.Errorf("foreign link was modified: %q", got)
	}
}

func TestEnsureRefusesRegularFile(t *testing.T) {
	root, owned := setup(t)
	binDir := t.TempDir()
	linkPath := filepath.Join(binDir, "ror")

	if err := os.WriteFile(linkPath, []byte("real file"), 0o755); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "installs", "ror", "latest", "ror")
	if _, err := Ensure(target, linkPath, owned, false); err == nil {
		t.Fatal("expected refusal for a regular file")
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("regular file was replaced with a symlink")
	}
}

func TestEnsureForceOverrides(t *testing.T) {
	root, owned := setup(t)
	binDir := t.TempDir()
	linkPath := filepath.Join(binDir, "ror")

	if err := os.Symlink("/etc/hosts", linkPath); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "installs", "ror", "latest", "ror")
	if _, err := Ensure(target, linkPath, owned, true); err != nil {
		t.Fatalf("Ensure with force: %v", err)
	}

	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Errorf("link = %q, want %q", got, target)
	}
}

func TestEnsureReplacesOirOwnedLink(t *testing.T) {
	root, owned := setup(t)
	binDir := t.TempDir()
	linkPath := filepath.Join(binDir, "ror")

	old := filepath.Join(root, "installs", "ror", "v1", "ror")
	if err := os.Symlink(old, linkPath); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "installs", "ror", "v2", "ror")
	changed, err := Ensure(target, linkPath, owned, false)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !changed {
		t.Error("changed = false, want true")
	}

	got, _ := os.Readlink(linkPath)
	if got != target {
		t.Errorf("link = %q, want %q", got, target)
	}
}
