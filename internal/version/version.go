// Package version holds build-time version information injected via -ldflags.
package version

import "fmt"

// Overridden at build time (see Makefile and .goreleaser.yaml).
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a human readable version string.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}
