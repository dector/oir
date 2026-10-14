package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/dector/oir/internal/alias"
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

//go:embed fixtures/aliases.oini
var registryFixture []byte

// readRegistryAliases parses a temporary embedded official registry snapshot.
// TODO: Replace the fixture with the shared catalog/cache reader.
func readRegistryAliases() ([]alias.Entry, error) {
	return alias.Parse(registryFixture)
}

func runSearch(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 || strings.TrimSpace(cmd.Args().First()) == "" {
		return fmt.Errorf("usage: oir search <query>")
	}

	registryEntries, err := readRegistryAliases()
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

	type result struct {
		spec   string
		source string
	}
	entries := make(map[string]result)
	for _, e := range registryEntries {
		entries[e.Key] = result{spec: e.Spec, source: "official"}
	}
	for _, e := range userEntries {
		entries[e.Key] = result{spec: e.Spec, source: "user"}
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
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", name, e.spec, e.source); err != nil {
			return err
		}
	}
	return w.Flush()
}
