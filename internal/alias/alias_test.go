package alias

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetGetListRemove(t *testing.T) {
	f := Open(filepath.Join(t.TempDir(), "aliases"))

	if _, ok, err := f.Get("ror"); err != nil || ok {
		t.Fatalf("Get(ror) = %v, %v; want missing", ok, err)
	}

	if err := f.Set("ror", "gh:dector/ror"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := f.Set("aaa", "gh:owner/aaa@v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	value, ok, err := f.Get("ror")
	if err != nil || !ok || value != "gh:dector/ror" {
		t.Fatalf("Get(ror) = %q, %v, %v", value, ok, err)
	}

	entries, err := f.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 || entries[0].Key != "aaa" || entries[1].Key != "ror" {
		t.Fatalf("List() = %v, want sorted aaa, ror", entries)
	}

	removed, err := f.Remove("ror")
	if err != nil || !removed {
		t.Fatalf("Remove(ror) = %v, %v; want true", removed, err)
	}
	if _, ok, _ := f.Get("ror"); ok {
		t.Fatal("ror still present after Remove")
	}

	removed, err = f.Remove("ror")
	if err != nil || removed {
		t.Fatalf("Remove(ror) again = %v, %v; want false", removed, err)
	}
}

func TestSetRejectsInvalidInput(t *testing.T) {
	f := Open(filepath.Join(t.TempDir(), "aliases"))

	if err := f.Set("bad key", "gh:dector/ror"); err == nil {
		t.Error("Set accepted an invalid alias name")
	}
	if err := f.Set("ror", "not a spec"); err == nil {
		t.Error("Set accepted an invalid spec")
	}
}

func TestLoadRejectsMalformedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aliases")
	if err := os.WriteFile(path, []byte("ror gh:dector/ror\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := Open(path)
	if _, err := f.List(); err == nil {
		t.Error("List accepted a malformed line")
	}
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OIR_CONFIG_DIR", dir)

	f := Open(filepath.Join(dir, "aliases"))
	if err := f.Set("ror", "gh:dector/ror"); err != nil {
		t.Fatal(err)
	}

	sp, err := Resolve("ror")
	if err != nil {
		t.Fatalf("Resolve(ror): %v", err)
	}
	if sp.Owner != "dector" || sp.Repo != "ror" || sp.Version != "" {
		t.Fatalf("Resolve(ror) = %+v", sp)
	}

	sp, err = Resolve("ror@v1.2.3")
	if err != nil {
		t.Fatalf("Resolve(ror@v1.2.3): %v", err)
	}
	if sp.Version != "v1.2.3" {
		t.Fatalf("version = %q, want v1.2.3", sp.Version)
	}

	if _, err := Resolve("gh:owner/repo"); err != nil {
		t.Fatalf("Resolve(spec): %v", err)
	}

	if _, err := Resolve("missing"); err == nil || !strings.Contains(err.Error(), "no tool registered") {
		t.Fatalf("Resolve(missing) error = %v", err)
	}
}

func TestDefaultPathHonorsEnv(t *testing.T) {
	t.Setenv("OIR_CONFIG_DIR", "/cfg")
	if got, err := DefaultPath(); err != nil || got != filepath.Join("/cfg", "aliases") {
		t.Fatalf("DefaultPath() = %q, %v", got, err)
	}

	t.Setenv("OIR_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got, err := DefaultPath(); err != nil || got != filepath.Join("/xdg", "oir", "aliases") {
		t.Fatalf("DefaultPath() = %q, %v", got, err)
	}
}
