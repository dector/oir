package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchMockRegistry(t *testing.T) {
	t.Setenv("OIR_CONFIG_DIR", t.TempDir())

	for _, command := range []string{"search", "s"} {
		t.Run(command, func(t *testing.T) {
			got := strings.Fields(run(t, command, "O"))
			want := "oir gh:dector/oir official ror gh:dector/ror official"
			if strings.Join(got, " ") != want {
				t.Fatalf("got %v, want %q", got, want)
			}
		})
	}
}

func TestSearchMatchesNamesOnly(t *testing.T) {
	t.Setenv("OIR_CONFIG_DIR", t.TempDir())

	if got := run(t, "search", "dector"); got != "" {
		t.Fatalf("search matched a target spec: %q", got)
	}
	if got := strings.Fields(run(t, "search", "OR")); strings.Join(got, " ") != "ror gh:dector/ror official" {
		t.Fatalf("substring search = %v", got)
	}
}

func TestSearchUserAliasesOverrideRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OIR_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "aliases"), []byte("ror = gh:example/custom\nother = gh:example/other\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(strings.Fields(run(t, "search", "o")), " ")
	want := "oir gh:dector/oir official other gh:example/other user ror gh:example/custom user"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSearchRequiresOneQuery(t *testing.T) {
	for _, args := range [][]string{{"search"}, {"search", ""}, {"search", " "}, {"search", "oir", "ror"}} {
		if err := runErr(t, args...); err == nil || !strings.Contains(err.Error(), "usage: oir search <query>") {
			t.Errorf("%v: got error %v", args, err)
		}
	}
}

func TestSearchReportsInvalidUserAliases(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OIR_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "aliases"), []byte("invalid line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runErr(t, "search", "oir"); err == nil {
		t.Fatal("expected invalid alias file error")
	}
}

func TestHelpListsSearch(t *testing.T) {
	if got := run(t, "help"); !strings.Contains(got, "search, s") {
		t.Fatalf("help missing search command: %s", got)
	}
}
