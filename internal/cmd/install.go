package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dector/oir/internal/catalog"
	"github.com/dector/oir/internal/gh"
	"github.com/dector/oir/internal/install"
	"github.com/dector/oir/internal/registry"
	"github.com/dector/oir/internal/spec"
	"github.com/dector/oir/internal/store"
	"github.com/dector/oir/internal/style"
	"github.com/urfave/cli/v3"
)

func newInstallCommand() *cli.Command {
	return &cli.Command{
		Name:      "install",
		Aliases:   []string{"i"},
		Usage:     "Install a full release bundle (use --exe-only for just the binary)",
		ArgsUsage: "gh:<owner>/<repo> | <alias>",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "refresh", Aliases: []string{"r"}, Usage: "Refresh all registries before installing (abort on any failure)"},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Reinstall and replace links that oir already owns",
			},
			&cli.BoolFlag{
				Name:  "no-verify",
				Usage: "Skip checksum verification (unsafe)",
			},
			&cli.BoolFlag{
				Name:  "full",
				Usage: "Keep the whole release archive (the default)",
			},
			&cli.BoolFlag{
				Name:    "exe-only",
				Aliases: []string{"only-binary"},
				Usage:   "Install only the executable; existing package files are kept unless --clear is used",
			},
			&cli.BoolFlag{
				Name:  "clear",
				Usage: "With --exe-only, remove existing package files instead of keeping them",
			},
			&cli.StringFlag{
				Name:  "bin",
				Usage: "Directory to symlink the binary into",
				Value: defaultBinDir(),
			},
		},
		Action: runInstall,
	}
}

func runInstall(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) == 0 {
		return fmt.Errorf("missing tool spec, for example: oir install gh:dector/ror")
	}
	if len(args) > 1 {
		return fmt.Errorf("expected a single tool spec, got %d arguments", len(args))
	}
	if cmd.Bool("full") && cmd.Bool("exe-only") {
		return fmt.Errorf("--full and --exe-only are mutually exclusive")
	}
	if cmd.Bool("clear") && !cmd.Bool("exe-only") {
		return fmt.Errorf("--clear requires --exe-only")
	}

	warnRootBinDir(cmd, os.Geteuid())

	resolved, err := resolveInstallArgument(ctx, cmd, args[0])
	if err != nil {
		return err
	}

	sp := resolved.Spec
	fmt.Fprintf(cmd.Writer, "resolving %s", style.Name(sp.String()))
	if resolved.Source == "user" {
		fmt.Fprint(cmd.Writer, " (user alias)")
	} else if resolved.Source != "" {
		fmt.Fprintf(cmd.Writer, " (registry: %s)", resolved.Source)
	}
	fmt.Fprintln(cmd.Writer)

	res, err := installResolved(ctx, cmd, sp)
	if err != nil {
		return err
	}

	// The installer reports the skip for an up-to-date tool; claiming
	// "installed" here would contradict it.
	if !res.UpToDate {
		fmt.Fprintf(cmd.Writer, "%s %s %s\n", style.Success("installed"), style.Name(res.Spec.String()), res.Version)
	}

	fmt.Fprintf(cmd.Writer, "  %s %s\n  %s %s\n",
		style.Muted("binary"), res.Binary,
		style.Muted("link  "), res.Link)

	if !pathHasDir(os.Getenv("PATH"), filepath.Dir(res.Link)) {
		fmt.Fprintf(cmd.ErrWriter, "%s %s is not on your PATH\n", style.Warn("warning:"), filepath.Dir(res.Link))
	}

	return nil
}

func resolveInstallArgument(ctx context.Context, cmd *cli.Command, raw string) (catalog.Resolution, error) {
	// Explicit refresh runs even for direct specs and user aliases.
	var c *catalog.Catalog
	var err error
	if cmd.Bool("refresh") {
		c, err = commandCatalog(cmd)
		if err != nil {
			return catalog.Resolution{}, err
		}
		if err := refreshCatalog(ctx, cmd, c); err != nil {
			return catalog.Resolution{}, err
		}
	}
	if catalog.Explicit(raw) {
		sp, err := spec.Parse(raw)
		return catalog.Resolution{Spec: sp}, err
	}
	f, err := openAliases()
	if err != nil {
		return catalog.Resolution{}, err
	}
	users, err := f.List()
	if err != nil {
		return catalog.Resolution{}, err
	}
	if c == nil {
		c, err = commandCatalog(cmd)
		if err != nil {
			return catalog.Resolution{}, err
		}
	}
	return c.Resolve(ctx, raw, users)
}

// This boundary accepts only a concrete spec; provenance stays in the CLI.
var installResolved = func(ctx context.Context, cmd *cli.Command, sp spec.Spec) (*install.Result, error) {
	in, err := newInstaller(cmd)
	if err != nil {
		return nil, err
	}
	return in.Run(ctx, sp)
}

func newBackends() registry.Backends {
	return registry.Backends{
		string(spec.BackendGitHub): gh.New(),
	}
}

// newInstaller builds an installer from the shared command flags. The store
// root is resolved here and passed as an option; nothing downstream chooses it.
func newInstaller(cmd *cli.Command) (*install.Installer, error) {
	storeDir, err := store.DefaultDir()
	if err != nil {
		return nil, err
	}

	return install.New(install.Options{
		Backends:   newBackends(),
		StoreDir:   storeDir,
		BinDir:     cmd.String("bin"),
		Platform:   registry.CurrentPlatform(),
		NoVerify:   cmd.Bool("no-verify"),
		Full:       cmd.Bool("full"),
		OnlyBinary: cmd.Bool("exe-only"),
		Clear:      cmd.Bool("clear"),
		Force:      cmd.Bool("force"),
		Stdout:     cmd.Writer,
		Stderr:     cmd.ErrWriter,
	}), nil
}

func warnRootBinDir(cmd *cli.Command, uid int) {
	if uid == 0 && !cmd.IsSet("bin") {
		fmt.Fprintf(cmd.ErrWriter, "%s running as root; binary links will be installed in /usr/local/bin (override with --bin)\n", style.Warn("warning:"))
	}
}

func defaultBinDir() string {
	return defaultBinDirForUID(os.Geteuid())
}

func defaultBinDirForUID(uid int) string {
	if uid == 0 {
		return "/usr/local/bin"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".local", "bin")
}

func pathHasDir(pathEnv, dir string) bool {
	dir = filepath.Clean(dir)
	for _, p := range filepath.SplitList(pathEnv) {
		if p == "" {
			continue
		}
		if filepath.Clean(p) == dir {
			return true
		}
	}

	return false
}
