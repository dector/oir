package cmd

import (
	"fmt"
	"github.com/dector/oir/internal/catalog"
	"github.com/urfave/cli/v3"
)

// Catalog construction is shared by all registry consumers. Tests substitute
// hermetic HTTP sources without changing the production registry list.
var makeCatalog = catalog.New

func commandCatalog(cmd *cli.Command) (*catalog.Catalog, error) {
	return makeCatalog(catalog.Options{Warn: func(err error) { fmt.Fprintf(cmd.ErrWriter, "warning: %v; using cached registry\n", err) }})
}
