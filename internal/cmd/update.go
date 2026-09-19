package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/dector/oir/internal/spec"
	"github.com/dector/oir/internal/store"
	"github.com/dector/oir/internal/style"
	"github.com/urfave/cli/v3"
)

func newUpdateCommand() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Aliases:   []string{"up"},
		Usage:     "Update every installed tool to its latest release",
		UsageText: "oir update [flags]",
		Flags: []cli.Flag{
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
		Action: runUpdate,
	}
}

func runUpdate(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() > 0 {
		return fmt.Errorf("update takes no arguments, got %d", cmd.Args().Len())
	}

	st, err := store.Default()
	if err != nil {
		return err
	}

	installed, err := st.List()
	if err != nil {
		return err
	}

	keys := uniqueKeys(installed)
	if len(keys) == 0 {
		fmt.Fprintln(cmd.Writer, style.Muted("no tools installed"))

		return nil
	}

	in, err := newInstaller(cmd)
	if err != nil {
		return err
	}

	var updated, current int
	var failed []string

	for _, key := range keys {
		sp, err := spec.FromKey(key)
		if err != nil {
			fmt.Fprintf(cmd.ErrWriter, "%s: %s\n", style.Name(key), style.Error(err.Error()))
			failed = append(failed, key)

			continue
		}

		res, err := in.Run(ctx, sp)
		if err != nil {
			fmt.Fprintf(cmd.ErrWriter, "%s: %s\n", style.Name(sp.String()), style.Error(err.Error()))
			failed = append(failed, sp.String())

			continue
		}

		if res.UpToDate {
			current++
		} else {
			updated++
			fmt.Fprintf(cmd.Writer, "%s %s to %s\n", style.Success("updated"), style.Name(sp.String()), res.Version)
		}
	}

	fmt.Fprintf(cmd.Writer, "\n%s, %d up to date", style.Success(fmt.Sprintf("%d updated", updated)), current)
	if len(failed) > 0 {
		fmt.Fprintf(cmd.Writer, ", %s", style.Error(fmt.Sprintf("%d failed: %s", len(failed), strings.Join(failed, ", "))))
	}
	fmt.Fprintln(cmd.Writer)

	if len(failed) > 0 {
		return fmt.Errorf("%d tool(s) failed to update", len(failed))
	}

	return nil
}

// uniqueKeys returns the distinct store keys in list order. List is already
// sorted, so keys only need to be deduplicated against their neighbour.
func uniqueKeys(list []store.Installed) []string {
	keys := make([]string, 0, len(list))
	for _, it := range list {
		if len(keys) == 0 || keys[len(keys)-1] != it.Key {
			keys = append(keys, it.Key)
		}
	}

	return keys
}
