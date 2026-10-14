package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/dector/oir/internal/catalog"
	"github.com/dector/oir/internal/install"
	"github.com/dector/oir/internal/spec"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestInstallResolutionAndProvenance(t *testing.T) {
	useTestCatalog(t)
	dir := t.TempDir()
	t.Setenv("OIR_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "aliases"), []byte("custom = gh:user/custom@old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old := installResolved
	t.Cleanup(func() { installResolved = old })
	for _, tc := range []struct {
		args               []string
		target, provenance string
	}{
		{[]string{"i", "ror@v2"}, "github:dector/ror@v2", "registry: official"},
		{[]string{"i", "custom@v3"}, "github:user/custom@v3", "user alias"},
		{[]string{"i", "custom@release/v1"}, "github:user/custom@release/v1", "user alias"},
		{[]string{"i", "-r", "custom"}, "github:user/custom@old", "user alias"},
	} {
		called := false
		installResolved = func(_ context.Context, _ *cli.Command, sp spec.Spec) (*install.Result, error) {
			called = true
			if sp.String() != tc.target {
				t.Fatalf("spec=%s", sp.String())
			}
			return &install.Result{Spec: sp, UpToDate: true, Link: "/bin/tool"}, nil
		}
		got := run(t, tc.args...)
		if !called || !strings.Contains(got, tc.provenance) {
			t.Fatalf("output: %s", got)
		}
	}
}

func TestInstallExplicitBypassesLookup(t *testing.T) {
	t.Setenv("OIR_CONFIG_DIR", t.TempDir())
	oldCatalog := makeCatalog
	oldInstall := installResolved
	t.Cleanup(func() { makeCatalog = oldCatalog; installResolved = oldInstall })
	makeCatalog = func(catalog.Options) (*catalog.Catalog, error) {
		t.Fatal("explicit spec consulted catalog")
		return nil, nil
	}
	installResolved = func(_ context.Context, _ *cli.Command, sp spec.Spec) (*install.Result, error) {
		return &install.Result{Spec: sp, UpToDate: true, Link: "/bin/tool"}, nil
	}
	if got := run(t, "i", "gh:direct/tool@v1"); strings.Contains(got, "registry:") {
		t.Fatal(got)
	}
}

func TestInstallRefreshStrictFailure(t *testing.T) {
	fail := false
	hits := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if fail {
			http.Error(w, "offline", 503)
			return
		}
		if strings.HasSuffix(r.URL.Path, "index.kdl") {
			w.Write([]byte("version \"1\"\naliases file=\"aliases.oini\" type=\"oir-aliases\""))
		} else {
			w.Write([]byte("tool = gh:o/tool"))
		}
	}))
	defer s.Close()
	useCatalogSources(t, []catalog.Source{{Name: "one", IndexURL: s.URL + "/one/index.kdl"}, {Name: "two", IndexURL: s.URL + "/two/index.kdl"}})
	run(t, "refresh")
	fail = true
	hits = 0
	old := installResolved
	t.Cleanup(func() { installResolved = old })
	installResolved = func(context.Context, *cli.Command, spec.Spec) (*install.Result, error) {
		t.Fatal("installation ran after refresh failure")
		return nil, errors.New("unexpected")
	}
	for _, flag := range []string{"--refresh", "-r"} {
		for _, target := range []string{"tool", "gh:direct/tool"} {
			before := hits
			if err := runErr(t, "i", flag, target); err == nil {
				t.Fatal("refresh failure ignored")
			}
			if hits-before != 2 {
				t.Fatal("did not refresh all registries")
			}
		}
	}
}

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
