package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dector/oir/internal/alias"
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
		Usage:     "Install a tool from a GitHub release",
		ArgsUsage: "gh:<owner>/<repo> | <alias>",
		Flags: []cli.Flag{
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
				Usage: "Keep the whole release archive, not just the binary",
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

	sp, err := alias.Resolve(args[0])
	if err != nil {
		return err
	}

	in, err := newInstaller(cmd)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.Writer, "resolving %s\n", style.Name(sp.String()))

	res, err := in.Run(ctx, sp)
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
		Backends: newBackends(),
		StoreDir: storeDir,
		BinDir:   cmd.String("bin"),
		Platform: registry.CurrentPlatform(),
		NoVerify: cmd.Bool("no-verify"),
		Full:     cmd.Bool("full"),
		Force:    cmd.Bool("force"),
		Stdout:   cmd.Writer,
		Stderr:   cmd.ErrWriter,
	}), nil
}

func defaultBinDir() string {
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
