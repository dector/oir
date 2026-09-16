// Command oir installs developer tools from GitHub.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dector/oir/internal/cmd"
)

func main() {
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "oir: %v\n", err)
		os.Exit(1)
	}
}
