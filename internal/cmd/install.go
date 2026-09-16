package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dector/oir/internal/gh"
	"github.com/dector/oir/internal/install"
	"github.com/dector/oir/internal/spec"
	"github.com/dector/oir/internal/store"
	"github.com/urfave/cli/v3"
)

func newInstallCommand() *cli.Command {
	return &cli.Command{
		Name:      "install",
		Aliases:   []string{"i"},
		Usage:     "Install a tool from a GitHub release",
		ArgsUsage: "gh:<owner>/<repo>",
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

	sp, err := spec.Parse(args[0])
	if err != nil {
		return err
	}

	st, err := store.Default()
	if err != nil {
		return err
	}

	binDir := cmd.String("bin")
	in := &install.Installer{
		Client:   gh.NewClient(),
		Store:    st,
		BinDir:   binDir,
		Platform: gh.CurrentPlatform(),
		NoVerify: cmd.Bool("no-verify"),
		Force:    cmd.Bool("force"),
		Stdout:   cmd.Writer,
		Stderr:   cmd.ErrWriter,
	}

	fmt.Fprintf(cmd.Writer, "resolving %s\n", sp)

	res, err := in.Run(ctx, sp)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.Writer, "installed %s %s\n  binary %s\n  link   %s\n",
		res.Spec, res.Version, res.Binary, res.Link)

	if !pathHasDir(os.Getenv("PATH"), filepath.Dir(res.Link)) {
		fmt.Fprintf(cmd.ErrWriter, "warning: %s is not on your PATH\n", filepath.Dir(res.Link))
	}

	return nil
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
