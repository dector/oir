package cmd

import (
	"regexp"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// helpSample is a piece of rendered help text covering every styled section.
const helpSample = `NAME:
   oir install - Install a tool

USAGE:
   oir install [options] gh:<owner>/<repo> | <alias>

COMMANDS:
   install, i  Install a tool
   alias, a    Manage tool aliases

GLOBAL OPTIONS:
   --help, -h  show help
`

// ansiPattern matches SGR escape sequences.
var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func withColor(t *testing.T) {
	t.Helper()

	noColor := color.NoColor

	color.NoColor = false
	t.Cleanup(func() { color.NoColor = noColor })
}

func TestColorizeHelpStylesSections(t *testing.T) {
	withColor(t)

	got := colorizeHelp(helpSample)

	for _, want := range []string{
		helpHeadingStyle.Sprint("NAME:"),
		helpHeadingStyle.Sprint("GLOBAL OPTIONS:"),
		helpNameStyle.Sprint("   oir install"),
		helpNameStyle.Sprint("install, i"),
		helpNameStyle.Sprint("--help, -h"),
		helpArgStyle.Sprint("<owner>"),
		helpArgStyle.Sprint("[options]"),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("styled help missing %q:\n%s", want, got)
		}
	}

	if plain := "  Install a tool"; !strings.Contains(got, plain) {
		t.Errorf("command description should stay unstyled, got:\n%s", got)
	}
}

func TestColorizeHelpKeepsLayout(t *testing.T) {
	withColor(t)

	if got := stripANSI(colorizeHelp(helpSample)); got != helpSample {
		t.Fatalf("styling changed the text:\ngot:\n%s\nwant:\n%s", got, helpSample)
	}
}

func TestColorizeHelpPlainWhenColorDisabled(t *testing.T) {
	noColor := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = noColor })

	if got := colorizeHelp(helpSample); got != helpSample {
		t.Fatalf("got %q, want %q", got, helpSample)
	}
}

func TestColorizeHelpStylesLabelsOnce(t *testing.T) {
	withColor(t)

	got := colorizeHelp(helpSample)

	if want := helpHeadingStyle.Sprint("GLOBAL OPTIONS:"); strings.Count(got, want) != 1 {
		t.Fatalf("GLOBAL OPTIONS: styled %d times, want 1:\n%s", strings.Count(got, want), got)
	}
	if nested := helpHeadingStyle.Sprint("OPTIONS:"); strings.Contains(got, nested) {
		t.Fatalf("OPTIONS: styled inside GLOBAL OPTIONS:\n%s", got)
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
		name, rest, ok := splitColumns(tt.body)
		if name != tt.name || rest != tt.rest || ok != tt.ok {
			t.Errorf("splitColumns(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.body, name, rest, ok, tt.name, tt.rest, tt.ok)
		}
	}
}
