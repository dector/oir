// Package cmd wires up the oir command-line interface.
//
// The root command is intentionally small. Add new commands to the Commands
// slice in New. The "help" command (alias "h") is provided by urfave/cli.
package cmd

import (
	"context"
	"sync"

	"github.com/urfave/cli/v3"
)

// colorOnce guards the one-time restyling of the shared urfave/cli help
// templates, which must not be wrapped in color codes more than once.
var colorOnce sync.Once

// Run builds and executes the oir CLI with the given arguments.
func Run(ctx context.Context, args []string) error {
	return New().Run(ctx, args)
}

// New builds the root command.
func New() *cli.Command {
	colorOnce.Do(colorHelp)

	return &cli.Command{
		Name:      "oir",
		Usage:     "install developer tools from GitHub",
		UsageText: "oir <command> [arguments]",
		Commands: []*cli.Command{
			newInstallCommand(),
			newUpdateCommand(),
			newAliasCommand(),
			newVersionCommand(),
		},
	}
}
