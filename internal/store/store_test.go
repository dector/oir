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

func TestHasIsFalseForMissing(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	if s.Has("github/x/y", "latest") {
		t.Error("Has() = true for a missing install")
	}
}

func TestAdoptIsAtomicAndReplaces(t *testing.T) {
	key, version := "github/dector/ror", "latest"
	s := &Store{Root: t.TempDir()}

	stage := func(content string) string {
		t.Helper()
		dir, err := s.Staging(key)
		if err != nil {
			t.Fatalf("Staging: %v", err)
		}
		bin := filepath.Join(dir, "ror")
		if err := os.WriteFile(bin, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}

		return dir
	}

	if err := s.Adopt(key, version, stage("v1")); err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	final := s.BinaryPath(key, version, "ror")
	body, err := os.ReadFile(final)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "v1" {
		t.Fatalf("got %q, want v1", body)
	}
	if !s.Has(key, version) {
		t.Fatal("Has() = false after Adopt")
	}

	// Replace with a new build, as happens when a "latest" tag is republished.
	if err := s.Adopt(key, version, stage("v2")); err != nil {
		t.Fatalf("Adopt (replace): %v", err)
	}
	if body, err = os.ReadFile(final); err != nil || string(body) != "v2" {
		t.Fatalf("got %q, %v, want v2", body, err)
	}

	// No staging or trash directories may be left behind.
	entries, err := os.ReadDir(s.toolDir(key))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != version {
		t.Fatalf("unexpected leftovers in install dir: %v", entries)
	}
}

func TestList(t *testing.T) {
	s := &Store{Root: t.TempDir()}

	// A version with metadata, and one installed before metadata existed.
	write := func(key, version, name string, meta *Meta) {
		t.Helper()
		dir := s.Dir(key, version)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if meta != nil {
			if err := s.WriteMeta(key, version, *meta); err != nil {
				t.Fatal(err)
			}
		}
	}

	write("github/o/tool", "v2", "tool", &Meta{Asset: "tool.tar.gz"})
	write("github/o/tool", "v1", "tool", nil)
	write("github/o/other", "latest", "other", &Meta{Asset: "other.tar.gz"})

	// Staging leftovers must not be reported.
	if err := os.MkdirAll(filepath.Join(s.Root, "installs", "github", "o", "tool", ".staging-x"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := []Installed{
		{Key: "github/o/other", Version: "latest", Meta: Meta{Asset: "other.tar.gz"}},
		{Key: "github/o/tool", Version: "v1"},
		{Key: "github/o/tool", Version: "v2", Meta: Meta{Asset: "tool.tar.gz"}},
	}
	if len(got) != len(want) {
		t.Fatalf("List len = %d (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestListMissingRoot(t *testing.T) {
	s := &Store{Root: filepath.Join(t.TempDir(), "nope")}

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List = %v, want empty", got)
	}
}

func TestMetaRoundTrip(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	key, version := "github/o/tool", "latest"

	if _, ok, err := s.ReadMeta(key, version); err != nil || ok {
		t.Fatalf("ReadMeta on missing = (%v, %v), want (false, nil)", ok, err)
	}

	if err := os.MkdirAll(s.Dir(key, version), 0o755); err != nil {
		t.Fatal(err)
	}

	want := Meta{Asset: "tool-linux-amd64.tar.gz", Digest: "sha256:abc", AssetID: 7}
	if err := s.WriteMeta(key, version, want); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	got, ok, err := s.ReadMeta(key, version)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if !ok {
		t.Fatal("ReadMeta ok = false after WriteMeta")
	}
	if got != want {
		t.Errorf("ReadMeta = %+v, want %+v", got, want)
	}
}

func TestListFindsPackageInstalls(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	key, version := "github/o/pi", "v1.0.0"

	dir := s.Dir(key, version)
	if err := os.MkdirAll(filepath.Join(dir, "theme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "theme", "dark.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta := Meta{Asset: "pi-linux-x64.tar.gz", Binary: "pi", Full: true}
	if err := s.WriteMeta(key, version, meta); err != nil {
		t.Fatal(err)
	}

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := []Installed{{Key: key, Version: version, Meta: meta}}
	if len(got) != len(want) {
		t.Fatalf("List = %+v, want %+v", got, want)
	}
	if got[0] != want[0] {
		t.Errorf("List[0] = %+v, want %+v", got[0], want[0])
	}
}
