// Command oir installs developer tools from GitHub.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dector/oir/internal/cmd"
	"github.com/dector/oir/internal/style"
)

func main() {
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Error("oir:"), err)
		os.Exit(1)
	}
}
