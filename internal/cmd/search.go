package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/dector/oir/internal/catalog"
	"github.com/urfave/cli/v3"
)

func newSearchCommand() *cli.Command {
	return &cli.Command{
		Name:      "search",
		Aliases:   []string{"s"},
		Usage:     "Search tool aliases",
		ArgsUsage: "<query>",
		Action:    runSearch,
	}
}

func runSearch(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 || strings.TrimSpace(cmd.Args().First()) == "" {
		return fmt.Errorf("usage: oir search <query>")
	}

	c, err := commandCatalog(cmd)
	if err != nil {
		return err
	}
	registryEntries, err := c.Entries(ctx)
	if err != nil {
		return err
	}
	f, err := openAliases()
	if err != nil {
		return err
	}
	userEntries, err := f.List()
	if err != nil {
		return err
	}

	entries := make(map[string]catalog.Entry)
	for _, e := range catalog.Merge(userEntries, registryEntries) {
		entries[e.Key] = e
	}

	query := strings.ToLower(cmd.Args().First())
	var names []string
	for name := range entries {
		if strings.Contains(strings.ToLower(name), query) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(cmd.Writer, 0, 0, 2, ' ', 0)
	for _, name := range names {
		e := entries[name]
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", name, e.Spec, e.Source); err != nil {
			return err
		}
	}
	return w.Flush()
}
