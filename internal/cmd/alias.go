package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/dector/oir/internal/alias"
	"github.com/urfave/cli/v3"
)

func newAliasCommand() *cli.Command {
	return &cli.Command{
		Name:      "alias",
		Aliases:   []string{"a"},
		Usage:     "Manage tool aliases",
		UsageText: "oir alias <command> [arguments]",
		Commands: []*cli.Command{
			newAliasGetCommand(),
			newAliasSetCommand(),
			newAliasRemoveCommand(),
		},
	}
}

func newAliasGetCommand() *cli.Command {
	return &cli.Command{
		Name:      "get",
		Usage:     "List aliases, or show one by key",
		ArgsUsage: "[key]",
		Action:    runAliasGet,
	}
}

func newAliasSetCommand() *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "Register an alias for a tool spec",
		ArgsUsage: "<key> <spec>",
		Action:    runAliasSet,
	}
}

func newAliasRemoveCommand() *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Aliases:   []string{"rm"},
		Usage:     "Delete an alias",
		ArgsUsage: "<key>",
		Action:    runAliasRemove,
	}
}

func openAliases() (*alias.File, error) {
	path, err := alias.DefaultPath()
	if err != nil {
		return nil, err
	}

	return alias.Open(path), nil
}

func runAliasGet(_ context.Context, cmd *cli.Command) error {
	f, err := openAliases()
	if err != nil {
		return err
	}

	args := cmd.Args().Slice()
	if len(args) > 1 {
		return fmt.Errorf("expected at most one key, got %d arguments", len(args))
	}

	if len(args) == 1 {
		value, ok, err := f.Get(args[0])
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("no alias registered with name %q", args[0])
		}

		fmt.Fprintln(cmd.Writer, value)

		return nil
	}

	entries, err := f.List()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(cmd.Writer, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\n", e.Key, e.Spec)
	}

	return w.Flush()
}

func runAliasSet(_ context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) != 2 {
		return fmt.Errorf("usage: oir alias set <key> <spec>")
	}

	f, err := openAliases()
	if err != nil {
		return err
	}
	if err := f.Set(args[0], args[1]); err != nil {
		return err
	}

	value, _, err := f.Get(args[0])
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.Writer, "%s = %s\n", args[0], value)

	return nil
}

func runAliasRemove(_ context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) != 1 {
		return fmt.Errorf("usage: oir alias remove <key>")
	}

	f, err := openAliases()
	if err != nil {
		return err
	}

	removed, err := f.Remove(args[0])
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("no alias registered with name %q", args[0])
	}

	fmt.Fprintf(cmd.Writer, "removed %s\n", args[0])

	return nil
}
