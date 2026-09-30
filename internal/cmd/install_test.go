package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestInstallModes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flags      []string
		binaryOnly bool
	}{
		{name: "default"},
		{name: "executable only", flags: []string{"--exe-only"}, binaryOnly: true},
		{name: "legacy binary flag", flags: []string{"--only-binary"}, binaryOnly: true},
		{name: "explicit full", flags: []string{"--full"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			cmd := newInstallCommand()
			cmd.Action = func(_ context.Context, c *cli.Command) error {
				in, err := newInstaller(c)
				if err != nil {
					return err
				}
				if in.OnlyBinary != tc.binaryOnly {
					t.Errorf("OnlyBinary = %v, want %v", in.OnlyBinary, tc.binaryOnly)
				}
				return nil
			}
			if err := cmd.Run(context.Background(), append([]string{"install"}, tc.flags...)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInstallModeValidation(t *testing.T) {
	for _, tc := range []struct {
		flags []string
		want  string
	}{
		{[]string{"--full", "--exe-only"}, "--full and --exe-only are mutually exclusive"},
		{[]string{"--clear"}, "--clear requires --exe-only"},
	} {
		cmd := newInstallCommand()
		args := append([]string{"install"}, tc.flags...)
		args = append(args, "gh:o/tool")
		err := cmd.Run(context.Background(), args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("flags %v: error = %v, want %q", tc.flags, err, tc.want)
		}
	}
}

func TestInstallHelpDescribesModes(t *testing.T) {
	got := run(t, "install", "--help")
	for _, want := range []string{"--exe-only", "full release bundle", "the default"} {
		if !strings.Contains(got, want) {
			t.Errorf("help missing %q: %s", want, got)
		}
	}
}
