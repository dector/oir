// Package version holds build-time version metadata for oir.
package version

import "fmt"

// These values are overwritten at build time with:
//
//	go build -ldflags "-X github.com/dector/oir/internal/version.Version=..."
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a single-line, human-readable version string.
func String() string {
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, Date)
}
