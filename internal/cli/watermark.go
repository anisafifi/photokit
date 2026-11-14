package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"photokit/internal/app"
	"photokit/internal/domain"
)

func newWatermarkCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watermark",
		Short: "Stamp a transparent PNG signature on photos",
		Long: `Composite a transparent PNG signature onto JPEG and PNG photos.

The original metadata (camera, lens, time, location, copyright…) is restored
on the result; orientation is applied to the pixels first, so placement is
always calculated on the image as it is displayed. RAW files can have their
metadata edited but are never rendered.`,
	}
	cmd.AddCommand(newWatermarkApplyCmd(e), newWatermarkPreviewCmd(e))
	return cmd
}

func newWatermarkApplyCmd(e *env) *cobra.Command {
	var (
		in        inputFlags
		wm        wmFlags
		format    string
		jobs      int
		out       string
		suffix    string
		quality   int
		overwrite bool
		dryRun    bool
	)
	cmd := &cobra.Command{
		Use:   "apply <file|dir>...",
		Short: "Apply the signature to copies of your photos",
		Example: `  photokit watermark apply ./photos --recursive \
    --image ./assets/signature.png --position bottom-right \
    --opacity 0.8 --scale 0.12 --margin 32 --output ./export`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			j, err := e.jobs(jobs)
			if err != nil {
				return err
			}
			cfg := wm.override(cmd, domain.DefaultWatermark())
			outDir := out
			if outDir == "" {
				outDir = e.cfg.Output
			}
			spec := app.Spec{
				Inputs: args, Recursive: in.recursive, Include: in.include, Exclude: in.exclude,
				Watermark: &cfg,
				Export:    app.ExportSpec{Quality: quality, Suffix: suffix},
				OutputDir: outDir, Overwrite: overwrite, DryRun: dryRun, Jobs: j,
			}
			return runBatch(cmd, e, format, func(p *app.Processor) (*domain.Report, error) {
				svc := &app.WatermarkService{Processor: p}
				return svc.Apply(cmd.Context(), spec)
			})
		},
	}
	in.add(cmd)
	wm.add(cmd)
	_ = cmd.MarkFlagRequired("image")
	addFormatFlag(cmd, &format)
	fl := cmd.Flags()
	fl.StringVarP(&out, "output", "o", "", "output directory (default ./export)")
	fl.StringVar(&suffix, "suffix", "", `text appended to output file names, e.g. "-signed"`)
	fl.IntVar(&quality, "quality", 0, "JPEG quality 1-100 (default 92)")
	fl.BoolVar(&overwrite, "overwrite", false, "replace existing output files")
	fl.BoolVar(&dryRun, "dry-run", false, "show what would happen without writing anything")
	fl.IntVar(&jobs, "jobs", 0, "number of files processed in parallel (default from config, 4)")
	return cmd
}

func newWatermarkPreviewCmd(e *env) *cobra.Command {
	var (
		wm        wmFlags
		out       string
		maxSize   int
		overwrite bool
	)
	cmd := &cobra.Command{
		Use:   "preview <image>",
		Short: "Render one image with the signature to check placement",
		Long: `Render a single JPEG or PNG with the signature and write the result to a file
(default ./preview.jpg or ./preview.png). The image is scaled down to --max-size
for a quick look; no metadata is written and the input is never modified.`,
		Example: `  photokit watermark preview photo.jpg --image assets/signature.png --position center -o preview.jpg`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := wm.override(cmd, domain.DefaultWatermark())
			svc := &app.WatermarkService{}
			path, err := svc.Preview(cmd.Context(), app.PreviewRequest{
				Input: args[0], Output: out, Watermark: cfg, MaxSize: maxSize, Overwrite: overwrite,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Preview written to %s\n", path)
			return nil
		},
	}
	wm.add(cmd)
	_ = cmd.MarkFlagRequired("image")
	fl := cmd.Flags()
	fl.StringVarP(&out, "output", "o", "", "preview file to write (.jpg or .png)")
	fl.IntVar(&maxSize, "max-size", 1600, "longest edge of the preview in pixels (0 = full size)")
	fl.BoolVar(&overwrite, "overwrite", false, "replace an existing preview file")
	return cmd
}
