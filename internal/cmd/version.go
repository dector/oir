package cmd

import (
	"context"
	"fmt"

	"github.com/dector/oir/internal/version"
	"github.com/urfave/cli/v3"
)

func newVersionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print version information",
		Action: func(_ context.Context, cmd *cli.Command) error {
			_, err := fmt.Fprintln(cmd.Writer, version.String())
			return err
		},
	}
}
