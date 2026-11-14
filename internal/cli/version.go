package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"photokit/internal/metadata"
	"photokit/internal/version"
)

func newVersionCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information and check for ExifTool",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "photokit %s\n", version.String())
			fmt.Fprintf(w, "platform %s/%s, %s\n", runtime.GOOS, runtime.GOARCH, runtime.Version())
			et, err := metadata.Detect(cmd.Context(), firstNonEmpty(e.exiftoolFlag, e.cfg.ExifToolPath))
			if err != nil {
				fmt.Fprintf(w, "exiftool: NOT AVAILABLE\n\n%v\n", err)
				return nil // informational: the version command itself succeeds
			}
			fmt.Fprintf(w, "exiftool %s (%s)\n", et.Version, et.Path)
			return nil
		},
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
