package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func runErr(t *testing.T, args ...string) error {
	t.Helper()

	var out bytes.Buffer

	root := New()
	root.Writer = &out
	root.ErrWriter = &out

	return root.Run(context.Background(), append([]string{"oir"}, args...))
}

func TestAliasSetGetRemove(t *testing.T) {
	t.Setenv("OIR_CONFIG_DIR", t.TempDir())

	if got := run(t, "a", "set", "ror", "gh:dector/ror"); !strings.Contains(got, "gh:dector/ror") {
		t.Fatalf("set output = %q", got)
	}
	if got := run(t, "alias", "get", "ror"); strings.TrimSpace(got) != "gh:dector/ror" {
		t.Fatalf("get output = %q", got)
	}
	if got := run(t, "a", "get"); !strings.Contains(got, "ror") {
		t.Fatalf("list output = %q", got)
	}
	if got := run(t, "a", "remove", "ror"); !strings.Contains(got, "removed") {
		t.Fatalf("remove output = %q", got)
	}
	if err := runErr(t, "a", "get", "ror"); err == nil || !strings.Contains(err.Error(), "no alias registered") {
		t.Fatalf("get after remove error = %v", err)
	}
}

func TestInstallUnknownAlias(t *testing.T) {
	t.Setenv("OIR_CONFIG_DIR", t.TempDir())

	err := runErr(t, "i", "ror")
	if err == nil || !strings.Contains(err.Error(), "no tool registered with name") {
		t.Fatalf("install unknown alias error = %v", err)
	}
}
