package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeVersion(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "latest"},
		{"latest", "latest"},
		{"v1.2.3", "v1.2.3"},
		{"release/1.0", "release-1.0"},
	}
	for _, tt := range tests {
		if got := SanitizeVersion(tt.in); got != tt.want {
			t.Errorf("SanitizeVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestOwns(t *testing.T) {
	s := &Store{Root: "/data/oir"}

	owned := []string{
		"/data/oir",
		"/data/oir/installs/github/dector/ror/latest/ror",
		"/data/oir/installs/../installs/x",
	}
	for _, p := range owned {
		if !s.Owns(p) {
			t.Errorf("Owns(%q) = false, want true", p)
		}
	}

	notOwned := []string{
		"/etc/hosts",
		"/data/other/ror",
		"/data/oir/../other/ror",
		"/data/oiroe/x",
	}
	for _, p := range notOwned {
		if s.Owns(p) {
			t.Errorf("Owns(%q) = true, want false", p)
		}
	}
}

func TestPlaceIsAtomicAndReplaces(t *testing.T) {
	root := t.TempDir()
	s := &Store{Root: root}

	src := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(src, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.Place("github/dector/ror", "latest", src, "ror")
	if err != nil {
		t.Fatalf("Place: %v", err)
	}

	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "v1" {
		t.Fatalf("got %q, want v1", body)
	}
	if !s.Has("github/dector/ror", "latest") {
		t.Fatal("Has() = false after Place")
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}

	// Replace with a new build, as happens when a "latest" tag is republished.
	if err := os.WriteFile(src, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Place("github/dector/ror", "latest", src, "ror"); err != nil {
		t.Fatalf("Place (replace): %v", err)
	}

	body, err = os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "v2" {
		t.Fatalf("got %q, want v2", body)
	}

	// No staging or trash directories may be left behind.
	entries, err := os.ReadDir(filepath.Dir(filepath.Dir(got)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "latest" {
		t.Fatalf("unexpected leftovers in install dir: %v", entries)
	}
}

func TestHasIsFalseForMissing(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	if s.Has("github/x/y", "latest") {
		t.Error("Has() = true for a missing install")
	}
}
