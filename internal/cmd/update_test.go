package cmd

import (
	"strings"
	"testing"

	"github.com/dector/oir/internal/store"
)

func TestUpdateWithNothingInstalled(t *testing.T) {
	t.Setenv("OIR_DATA_DIR", t.TempDir())

	if got := run(t, "update"); !strings.Contains(got, "no tools installed") {
		t.Fatalf("update output = %q", got)
	}
	if got := run(t, "up"); !strings.Contains(got, "no tools installed") {
		t.Fatalf("up output = %q", got)
	}
}

func TestUpdateRejectsArguments(t *testing.T) {
	t.Setenv("OIR_DATA_DIR", t.TempDir())

	err := runErr(t, "update", "gh:dector/ror")
	if err == nil || !strings.Contains(err.Error(), "takes no arguments") {
		t.Fatalf("update with argument error = %v", err)
	}
}

func TestUniqueKeys(t *testing.T) {
	got := uniqueKeys([]store.Installed{
		{Key: "github/o/a", Version: "v1"},
		{Key: "github/o/a", Version: "v2"},
		{Key: "github/o/b", Version: "latest"},
	})
	want := []string{"github/o/a", "github/o/b"}
	if len(got) != len(want) {
		t.Fatalf("uniqueKeys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("uniqueKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
