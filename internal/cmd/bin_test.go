package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestDefaultBinDirForUID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, tc := range []struct {
		name string
		uid  int
		want string
	}{
		{"root", 0, "/usr/local/bin"},
		{"regular user", 1000, filepath.Join(home, ".local", "bin")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultBinDirForUID(tc.uid); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWarnRootBinDir(t *testing.T) {
	for _, tc := range []struct {
		name string
		uid  int
		args []string
		warn bool
	}{
		{"root default", 0, nil, true},
		{"regular user", 1000, nil, false},
		{"root override", 0, []string{"--bin", "/tmp/bin"}, false},
		{"root explicit system bin", 0, []string{"--bin", "/usr/local/bin"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			cmd := &cli.Command{
				ErrWriter: &stderr,
				Flags:     []cli.Flag{&cli.StringFlag{Name: "bin", Value: "/usr/local/bin"}},
				Action: func(_ context.Context, cmd *cli.Command) error {
					warnRootBinDir(cmd, tc.uid)
					return nil
				},
			}
			if err := cmd.Run(context.Background(), append([]string{"test"}, tc.args...)); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(stderr.String(), "running as root; binary links will be installed in /usr/local/bin"); got != tc.warn {
				t.Fatalf("warning=%v, want %v; stderr: %s", got, tc.warn, stderr.String())
			}
		})
	}
}

func TestRootBinDirWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	if got := defaultBinDirForUID(0); got != "/usr/local/bin" {
		t.Fatalf("got %q, want /usr/local/bin", got)
	}
}
