package cli

import (
	"github.com/spf13/cobra"

	"photokit/internal/app"
	"photokit/internal/domain"
	"photokit/internal/output"
)

func newMetadataCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metadata",
		Short: "Read and edit EXIF / IPTC / XMP metadata",
	}
	cmd.AddCommand(newMetadataShowCmd(e), newMetadataSetCmd(e))
	return cmd
}

func newMetadataShowCmd(e *env) *cobra.Command {
	var (
		in     inputFlags
		format string
		jobs   int
	)
	cmd := &cobra.Command{
		Use:   "show <file|dir>...",
		Short: "Show only the copyright / descriptive metadata",
		Example: `  photokit metadata show photo.jpg
  photokit metadata show ./export --recursive --format json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			infos, err := inspect(cmd, e, args, in, &jobs, format)
			if err != nil {
				return err
			}
			return finishInspect(cmd, infos, format, output.Metadata)
		},
	}
	in.add(cmd)
	addFormatFlag(cmd, &format)
	cmd.Flags().IntVar(&jobs, "jobs", 0, "number of files read in parallel (default from config, 4)")
	return cmd
}

func newMetadataSetCmd(e *env) *cobra.Command {
	var (
		in        inputFlags
		md        metaFlags
		format    string
		jobs      int
		out       string
		suffix    string
		overwrite bool
		dryRun    bool
		inPlace   bool
	)
	cmd := &cobra.Command{
		Use:   "set <file|dir>...",
		Short: "Write creator, copyright and descriptive metadata to copies of your photos",
		Long: `Write metadata to every photo. Values are written consistently to EXIF, IPTC
and XMP so the standards never disagree.

By default PhotoKit leaves the originals untouched and writes modified copies
into the output directory (default ./export). Use --in-place to edit the
originals directly (this is the only command that can do so, and only when you
ask for it explicitly).

Only the fields you pass are changed; --keywords replaces the existing
keywords unless --append-keywords is given.`,
		Example: `  photokit metadata set ./photos --recursive \
    --creator "Your Name" --copyright "© 2026 Your Name" \
    --rights "All rights reserved" --keywords photography --keywords portrait

  photokit metadata set ./photos --in-place --creator "Your Name"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			j, err := e.jobs(jobs)
			if err != nil {
				return err
			}
			outDir := out
			if outDir == "" && !inPlace {
				outDir = e.cfg.Output
			}
			spec := app.Spec{
				Inputs: args, Recursive: in.recursive, Include: in.include, Exclude: in.exclude,
				Metadata: md.value(), AppendKeywords: md.appendKeywords,
				Export:    app.ExportSpec{Suffix: suffix},
				OutputDir: outDir, InPlace: inPlace, Overwrite: overwrite, DryRun: dryRun, Jobs: j,
			}
			return runBatch(cmd, e, format, func(p *app.Processor) (*domain.Report, error) {
				svc := &app.MetadataService{Engine: p.Engine, Processor: p}
				return svc.Set(cmd.Context(), spec)
			})
		},
	}
	in.add(cmd)
	md.add(cmd, true)
	addFormatFlag(cmd, &format)
	fl := cmd.Flags()
	fl.StringVarP(&out, "output", "o", "", "output directory (default ./export)")
	fl.StringVar(&suffix, "suffix", "", `text appended to output file names, e.g. "-copyright"`)
	fl.BoolVar(&overwrite, "overwrite", false, "replace existing output files")
	fl.BoolVar(&dryRun, "dry-run", false, "show what would happen without writing anything")
	fl.BoolVar(&inPlace, "in-place", false, "modify the original files instead of writing copies")
	fl.IntVar(&jobs, "jobs", 0, "number of files processed in parallel (default from config, 4)")
	return cmd
}
