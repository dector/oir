package cmd

import (
	"context"
	"fmt"

	"github.com/dector/oir/internal/catalog"
	"github.com/urfave/cli/v3"
)

func newRefreshCommand() *cli.Command {
	return &cli.Command{Name: "refresh", Usage: "Refresh all tool registries", Action: func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() != 0 {
			return fmt.Errorf("usage: oir refresh")
		}
		c, err := commandCatalog(cmd)
		if err != nil {
			return err
		}
		if err := refreshCatalog(ctx, cmd, c); err != nil {
			return err
		}
		fmt.Fprintln(cmd.Writer, "registries refreshed")
		return nil
	}}
}

func refreshCatalog(ctx context.Context, cmd *cli.Command, c *catalog.Catalog) error {
	if err := c.Refresh(ctx); err != nil {
		// Refresh attempts all sources and the aggregate contains every failure.
		fmt.Fprintln(cmd.ErrWriter, err)
		return fmt.Errorf("registry refresh failed")
	}
	return nil
}
