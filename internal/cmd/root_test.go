package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dector/oir/internal/version"
)

func run(t *testing.T, args ...string) string {
	t.Helper()

	var out bytes.Buffer

	root := New()
	root.Writer = &out
	root.ErrWriter = &out

	if err := root.Run(context.Background(), append([]string{"oir"}, args...)); err != nil {
		t.Fatalf("run %v: %v", args, err)
	}

	return out.String()
}

func TestVersionCommand(t *testing.T) {
	got := strings.TrimSpace(run(t, "version"))
	if want := version.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHelpListsCommands(t *testing.T) {
	got := run(t, "help")
	for _, want := range []string{"version", "help", "update"} {
		if !strings.Contains(got, want) {
			t.Errorf("help output missing %q:\n%s", want, got)
		}
	}
}
