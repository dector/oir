package cmd

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dector/oir/internal/style"
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
		style.Heading("NAME:"),
		style.Heading("GLOBAL OPTIONS:"),
		style.Name("   oir install"),
		style.Name("install, i"),
		style.Name("--help, -h"),
		style.Muted("<owner>"),
		style.Muted("[options]"),
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

	if want := style.Heading("GLOBAL OPTIONS:"); strings.Count(got, want) != 1 {
		t.Fatalf("GLOBAL OPTIONS: styled %d times, want 1:\n%s", strings.Count(got, want), got)
	}
	if nested := style.Heading("OPTIONS:"); strings.Contains(got, nested) {
		t.Fatalf("OPTIONS: styled inside GLOBAL OPTIONS:\n%s", got)
	}
}
