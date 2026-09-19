package style

import (
	"regexp"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// ansiPattern matches SGR escape sequences.
var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func withColor(t *testing.T) {
	t.Helper()

	noColor := color.NoColor

	color.NoColor = false
	t.Cleanup(func() { color.NoColor = noColor })
}

func TestStylesAddEscapeCodes(t *testing.T) {
	withColor(t)

	tests := []struct {
		label string
		got   string
	}{
		{"Heading", Heading("USAGE:")},
		{"Name", Name("install")},
		{"Muted", Muted("<repo>")},
		{"Success", Success("installed")},
		{"Warn", Warn("warning:")},
		{"Error", Error("boom")},
	}

	for _, tt := range tests {
		if !strings.Contains(tt.got, "\x1b[") {
			t.Errorf("%s did not add an escape code: %q", tt.label, tt.got)
		}
		if ansiPattern.ReplaceAllString(tt.got, "") == "" {
			t.Errorf("%s lost its text: %q", tt.label, tt.got)
		}
	}
}

func TestStylesPlainWhenColorDisabled(t *testing.T) {
	noColor := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = noColor })

	if got := Name("install"); got != "install" {
		t.Fatalf("got %q, want %q", got, "install")
	}
}

func TestSplitColumns(t *testing.T) {
	tests := []struct {
		body string
		name string
		rest string
		ok   bool
	}{
		{"install, i  Install a tool", "install, i", "  Install a tool", true},
		{"--bin string  Directory", "--bin string", "  Directory", true},
		{"--help, -h  show help", "--help, -h", "  show help", true},
		{"get", "", "", false},
		{"Files:", "", "", false},
		{"--force  Reinstall and replace links", "--force", "  Reinstall and replace links", true},
	}

	for _, tt := range tests {
		name, rest, ok := SplitColumns(tt.body)
		if name != tt.name || rest != tt.rest || ok != tt.ok {
			t.Errorf("SplitColumns(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.body, name, rest, ok, tt.name, tt.rest, tt.ok)
		}
	}
}
