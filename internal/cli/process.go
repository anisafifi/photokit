package cli

import (
	"github.com/spf13/cobra"

	"photokit/internal/app"
	"photokit/internal/domain"
	"photokit/internal/preset"
)

func newProcessCmd(e *env) *cobra.Command {
	var (
		in        inputFlags
		md        metaFlags
		wm        wmFlags
		format    string
		jobs      int
		presetArg string
		out       string
		suffix    string
		exportFmt string
		quality   int
		noWM      bool
		overwrite bool
		dryRun    bool
	)
	cmd := &cobra.Command{
		Use:   "process <file|dir>...",
		Short: "Run a preset: metadata + signature + export in one command",
		Long: `Process photos with a YAML preset. The preset describes the metadata to write,
the signature to apply and the export settings; everything runs through the
same services as the direct commands.

--preset takes a file path, or a bare name that is looked up in ./presets and
the preset directories from your config. Flags override the preset: metadata
flags replace the preset's values (--tag merges per tag, --remove adds to the preset's list),
watermark flags change individual signature settings, --output replaces
export.directory.

Originals are never modified: a preset can only write copies.`,
		Example: `  photokit process ./photos --preset ./presets/default.yaml --output ./export
  photokit process ./photos --preset default --dry-run
  photokit process ./photos -r --preset default --no-watermark --export-format jpeg --quality 85`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			j, err := e.jobs(jobs)
			if err != nil {
				return err
			}
			path, err := preset.Find(presetArg, e.cfg.SearchDirs())
			if err != nil {
				return err
			}
			p, err := preset.Load(path)
			if err != nil {
				return err
			}
			e.log.Debug("preset loaded", "path", path, "name", p.Name)

			flagMeta, err := md.value(cmd)
			if err != nil {
				return err
			}
			spec := app.Spec{
				Inputs: args, Recursive: in.recursive, Include: in.include, Exclude: in.exclude,
				Metadata:       p.Metadata.Override(flagMeta),
				AppendKeywords: false,
				Export: app.ExportSpec{
					Format: p.ExportFormat(), Quality: p.QualityOrDefault(), Suffix: p.Export.Suffix,
				},
				Overwrite: overwrite, DryRun: dryRun, Jobs: j,
			}

			// Watermark: preset, then flag overrides.
			base := p.WatermarkConfig()
			switch {
			case noWM:
				if wm.anyChanged(cmd) {
					return domain.Invalidf("--no-watermark cannot be combined with watermark settings")
				}
			case base != nil:
				cfg := wm.override(cmd, *base)
				spec.Watermark = &cfg
			case cmd.Flags().Changed("image"):
				cfg := wm.override(cmd, domain.DefaultWatermark())
				spec.Watermark = &cfg
			case wm.anyChanged(cmd):
				return domain.Invalidf("watermark flags need a signature: pass --image or enable watermark in the preset")
			}

			if cmd.Flags().Changed("export-format") {
				f, err := domain.ParseExportFormat(exportFmt)
				if err != nil {
					return err
				}
				spec.Export.Format = f
			}
			if cmd.Flags().Changed("quality") {
				spec.Export.Quality = quality
			}
			if cmd.Flags().Changed("suffix") {
				spec.Export.Suffix = suffix
			}

			switch {
			case out != "":
				spec.OutputDir = out
			case p.Export.Directory != "":
				spec.OutputDir = p.Export.Directory
			default:
				spec.OutputDir = e.cfg.Output
			}

			return runBatch(cmd, e, format, func(pr *app.Processor) (*domain.Report, error) {
				return pr.Run(cmd.Context(), spec)
			})
		},
	}
	in.add(cmd)
	md.add(cmd, false)
	wm.add(cmd)
	addFormatFlag(cmd, &format)
	fl := cmd.Flags()
	fl.StringVarP(&presetArg, "preset", "p", "", "preset file or name (required)")
	_ = cmd.MarkFlagRequired("preset")
	fl.StringVarP(&out, "output", "o", "", "output directory (overrides export.directory)")
	fl.StringVar(&suffix, "suffix", "", "text appended to output file names (overrides export.suffix)")
	fl.StringVar(&exportFmt, "export-format", "", "original, jpeg or png (overrides export.format)")
	fl.IntVar(&quality, "quality", 0, "JPEG quality 1-100 (overrides export.quality)")
	fl.BoolVar(&noWM, "no-watermark", false, "skip the signature even if the preset enables it")
	fl.BoolVar(&overwrite, "overwrite", false, "replace existing output files")
	fl.BoolVar(&dryRun, "dry-run", false, "show what would happen without writing anything")
	fl.IntVar(&jobs, "jobs", 0, "number of files processed in parallel (default from config, 4)")
	return cmd
}
