package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"photokit/internal/app"
	"photokit/internal/domain"
	"photokit/internal/output"
)

func newInfoCmd(e *env) *cobra.Command {
	var (
		in     inputFlags
		format string
		jobs   int
	)
	cmd := &cobra.Command{
		Use:   "info <file|dir>...",
		Short: "Show camera details and metadata of photos",
		Long: `Show format, size, camera, lens, capture date and the managed copyright
metadata (creator, copyright, rights, keywords…) of one or more photos.
Nothing is modified.`,
		Example: `  photokit info photo.jpg
  photokit info ./photos --recursive --format json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			infos, err := inspect(cmd, e, args, in, &jobs, format)
			if err != nil {
				return err
			}
			return finishInspect(cmd, infos, format, output.Infos)
		},
	}
	in.add(cmd)
	addFormatFlag(cmd, &format)
	cmd.Flags().IntVar(&jobs, "jobs", 0, "number of files read in parallel (default from config, 4)")
	return cmd
}

func inspect(cmd *cobra.Command, e *env, args []string, in inputFlags, jobs *int, format string) ([]domain.Info, error) {
	if _, err := domain.ParseOutputFormat(format); err != nil {
		return nil, err
	}
	j, err := e.jobs(*jobs)
	if err != nil {
		return nil, err
	}
	eng, err := e.Engine(cmd.Context())
	if err != nil {
		return nil, err
	}
	svc := &app.MetadataService{Engine: eng}
	return svc.Inspect(cmd.Context(), args, in.discover(), j)
}

func finishInspect(cmd *cobra.Command, infos []domain.Info, format string, print func(w io.Writer, infos []domain.Info, format string) error) error {
	f, _ := domain.ParseOutputFormat(format)
	if err := print(cmd.OutOrStdout(), infos, f); err != nil {
		return err
	}
	failed := 0
	for _, i := range infos {
		if i.Error != "" {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d file(s) could not be read", failed)
	}
	return nil
}
