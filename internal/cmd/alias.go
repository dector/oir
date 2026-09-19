package cmd

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/dector/oir/internal/alias"
	"github.com/dector/oir/internal/style"
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

	// Style after tabwriter has aligned the columns: escape codes would count
	// towards the column widths.
	var buf bytes.Buffer

	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\n", e.Key, e.Spec)
	}

	if err := w.Flush(); err != nil {
		return err
	}

	table := strings.TrimSuffix(buf.String(), "\n")
	if table == "" {
		return nil
	}

	for _, line := range strings.Split(table, "\n") {
		fmt.Fprintln(cmd.Writer, styleKey(line))
	}

	return nil
}

// styleKey styles the alias key in a rendered table row.
func styleKey(line string) string {
	key, spec, ok := style.SplitColumns(line)
	if !ok {
		return line
	}

	return style.Name(key) + spec
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

	fmt.Fprintf(cmd.Writer, "%s = %s\n", style.Name(args[0]), value)

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

	fmt.Fprintf(cmd.Writer, "%s %s\n", style.Success("removed"), style.Name(args[0]))

	return nil
}
