package alias

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	data := []byte("# Tools\r\n\r\n ror = gh:dector/old \r\noir=gh:dector/oir\r\nror = gh:dector/ror\r\n")
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{Key: "oir", Spec: "gh:dector/oir"}, {Key: "ror", Spec: "gh:dector/ror"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %v, want %v", got, want)
	}

	path := filepath.Join(t.TempDir(), "aliases.oini")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	fromFile, err := Open(path).List()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, fromFile) {
		t.Fatalf("Parse() = %v, File.List() = %v", got, fromFile)
	}
}

func TestParseDoesNotValidateUserEntries(t *testing.T) {
	data := []byte("bad name = not-a-spec\n")
	got, err := Parse(data)
	want := []Entry{{Key: "bad name", Spec: "not-a-spec"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %v, %v; want %v", got, err, want)
	}
	path := filepath.Join(t.TempDir(), "aliases")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = Open(path).List()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %v, %v; want %v", got, err, want)
	}
	if err := Validate(want[0].Key, want[0].Spec); err == nil {
		t.Fatal("Validate accepted invalid entry")
	}
}

func TestParseEmpty(t *testing.T) {
	got, err := Parse([]byte("# Empty snapshot\n\n"))
	if err != nil || len(got) != 0 {
		t.Fatalf("Parse() = %v, %v; want empty entries", got, err)
	}
}

func TestParseRejectsMalformedLines(t *testing.T) {
	for _, line := range []string{"no equals", "= gh:dector/oir", "oir =", "[section]", "; comment"} {
		t.Run(line, func(t *testing.T) {
			entries, err := Parse([]byte("oir = gh:dector/oir\n" + line))
			if err == nil || !strings.Contains(err.Error(), "aliases:2:") {
				t.Fatalf("Parse() error = %v, want line 2 error", err)
			}
			if entries != nil {
				t.Fatalf("Parse() returned partial entries: %v", entries)
			}
		})
	}
}
