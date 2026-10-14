package catalog

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/dector/oir/internal/alias"
)

func TestPublishedRegistry(t *testing.T) {
	// Copied verbatim from the repository's registry branch.
	data, err := os.ReadFile("testdata/index.kdl")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	want := Manifest{Version: "1", File: "aliases.oini", Type: "oir-aliases"}
	if m != want {
		t.Fatalf("got %+v, want %+v", m, want)
	}
	data, err = os.ReadFile("testdata/" + m.File)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ParseAliases(data)
	if err != nil {
		t.Fatal(err)
	}
	expected := []alias.Entry{{Key: "oir", Spec: "gh:dector/oir"}, {Key: "ror", Spec: "gh:dector/ror"}}
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("got %v, want %v", entries, expected)
	}
}

func TestManifestKDL(t *testing.T) {
	data := `// Registry comment
version "1"; aliases type="oir-aliases" file="tools.oini" // another comment
`
	got, err := ParseManifest([]byte(data))
	if err != nil || got.File != "tools.oini" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestInvalidManifest(t *testing.T) {
	tests := []struct{ name, data, message string }{
		{"empty", "", "version"},
		{"syntax", `version "1`, "index.kdl"},
		{"missing version", `aliases file="aliases.oini" type="oir-aliases"`, "version"},
		{"duplicate version", "version \"1\"\nversion \"1\"", "exactly one version"},
		{"numeric version", `version 1`, "string argument"},
		{"version properties", `version "1" extra="x"`, "version"},
		{"extra argument", `version "1" "2"`, "version"},
		{"missing aliases", `version "1"`, "exactly one aliases"},
		{"duplicate aliases", "version \"1\"\naliases file=\"a\" type=\"oir-aliases\"\naliases file=\"b\" type=\"oir-aliases\"", "exactly one aliases"},
		{"missing file", "version \"1\"\naliases type=\"oir-aliases\"", "requires"},
		{"missing type", "version \"1\"\naliases file=\"aliases.oini\"", "requires"},
		{"empty file", "version \"1\"\naliases file=\"\" type=\"oir-aliases\"", "requires"},
		{"empty type", "version \"1\"\naliases file=\"a\" type=\"\"", "requires"},
		{"nonstring property", "version \"1\"\naliases file=42 type=\"oir-aliases\"", "string"},
		{"unknown property", "version \"1\"\naliases file=\"a\" type=\"oir-aliases\" other=\"x\"", "unknown"},
		{"unknown node", "version \"1\"\nother", "unknown"},
		{"aliases argument", "version \"1\"\naliases \"a\"", "aliases"},
		{"children", "version \"1\"\naliases file=\"a\" type=\"oir-aliases\" { child; }", "leaf"},
		{"typed node", `(schema)version "1"`, "untyped"},
		{"typed value", `version (schema)"1"`, "string argument"},
		{"upgrade version", "version \"2\"\naliases file=\"a\" type=\"oir-aliases\"", "oir upgrade required"},
		{"upgrade type", "version \"1\"\naliases file=\"a\" type=\"new-format\"", "oir upgrade required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseManifest([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("got %+v, %v; want %q error", got, err, tt.message)
			}
			if got != (Manifest{}) {
				t.Fatalf("partial result: %+v", got)
			}
		})
	}
}

func TestUnsafeFilenames(t *testing.T) {
	for _, name := range []string{".", "..", "../aliases.oini", "a/b", "/aliases.oini", `a\b`, `C:\aliases.oini`, "C:aliases.oini", `\\server\aliases.oini`, "https://example.com/a", "//example.com/a", "%2e%2e", "a?b", "a#b", "a\x00b", "a\nb", "a b", "a.", "CON", "nul.oini", "LPT1.oini"} {
		t.Run(name, func(t *testing.T) {
			data := fmt.Sprintf("version \"1\"\naliases file=%q type=\"oir-aliases\"\n", name)
			if _, err := ParseManifest([]byte(data)); err == nil {
				t.Fatalf("accepted %q", name)
			}
		})
	}
}

func TestRegistryAliases(t *testing.T) {
	data := "# comment\n\nror = github:dector/ror@v1.2\noir = gh:dector/old\noir = gh:dector/oir\n"
	got, err := ParseAliases([]byte(data))
	want := []alias.Entry{{Key: "oir", Spec: "gh:dector/oir"}, {Key: "ror", Spec: "github:dector/ror@v1.2"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
	got, err = ParseAliases([]byte("# empty registry\n"))
	if err != nil || len(got) != 0 {
		t.Fatalf("empty: %v, %v", got, err)
	}
}

func TestInvalidRegistryAliases(t *testing.T) {
	for _, line := range []string{"broken", "= gh:dector/oir", "oir =", "[section]", "; comment", "bad name = gh:dector/oir", "../oir = gh:dector/oir", "oir@v1 = gh:dector/oir", "bad = not-a-spec", "bad = https:owner/repo", "bad = gh:owner/a/b", "bad = gh:/repo"} {
		t.Run(line, func(t *testing.T) {
			got, err := ParseAliases([]byte("oir = gh:dector/oir\n" + line))
			if err == nil || got != nil {
				t.Fatalf("got %v, %v; want error and no partial entries", got, err)
			}
		})
	}
}
