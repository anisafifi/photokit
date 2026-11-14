package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"photokit/internal/app"
	"photokit/internal/batch"
	"photokit/internal/domain"
	"photokit/internal/output"
)

// inputFlags select which files are processed.
type inputFlags struct {
	recursive bool
	include   []string
	exclude   []string
}

func (f *inputFlags) add(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.BoolVarP(&f.recursive, "recursive", "r", false, "include nested directories")
	fl.StringSliceVar(&f.include, "include", nil, "only process files matching these glob patterns (e.g. '*.jpg')")
	fl.StringSliceVar(&f.exclude, "exclude", nil, "skip files matching these glob patterns")
}

func (f *inputFlags) discover() batch.DiscoverOptions {
	return batch.DiscoverOptions{Recursive: f.recursive, Include: f.include, Exclude: f.exclude}
}

// metaFlags carry the metadata fields.
type metaFlags struct {
	creator, copyright, rights, description string
	title, credit, source, instructions     string
	keywords                                []string
	appendKeywords                          bool
}

func (f *metaFlags) add(cmd *cobra.Command, withAppend bool) {
	fl := cmd.Flags()
	fl.StringVar(&f.creator, "creator", "", "creator / photographer name")
	fl.StringVar(&f.copyright, "copyright", "", `copyright notice, e.g. "© 2026 Your Name"`)
	fl.StringVar(&f.rights, "rights", "", `usage terms, e.g. "All rights reserved"`)
	fl.StringVar(&f.description, "description", "", "caption / description")
	fl.StringVar(&f.title, "title", "", "title")
	fl.StringSliceVar(&f.keywords, "keywords", nil, "keyword (repeat the flag or separate with commas)")
	fl.StringVar(&f.credit, "credit", "", "credit line")
	fl.StringVar(&f.source, "source", "", "source")
	fl.StringVar(&f.instructions, "instructions", "", "special instructions")
	if withAppend {
		fl.BoolVar(&f.appendKeywords, "append-keywords", false, "add --keywords to the existing keywords instead of replacing them")
	}
}

func (f *metaFlags) value() domain.Metadata {
	return domain.Metadata{
		Creator: f.creator, Copyright: f.copyright, Rights: f.rights,
		Description: f.description, Title: f.title, Keywords: f.keywords,
		Credit: f.credit, Source: f.source, Instructions: f.instructions,
	}
}

// wmFlags carry the signature settings.
type wmFlags struct {
	image    string
	position string
	opacity  float64
	scale    float64
	margin   int
}

var wmNames = []string{"image", "position", "opacity", "scale", "margin"}

func (f *wmFlags) add(cmd *cobra.Command) {
	d := domain.DefaultWatermark()
	fl := cmd.Flags()
	fl.StringVar(&f.image, "image", "", "transparent PNG signature")
	fl.StringVar(&f.position, "position", d.Position, "anchor: top-left, top-center, top-right, center-left, center, center-right, bottom-left, bottom-center, bottom-right")
	fl.Float64Var(&f.opacity, "opacity", d.Opacity, "signature opacity, 0-1")
	fl.Float64Var(&f.scale, "scale", d.Scale, "signature width relative to the image width, 0-1")
	fl.IntVar(&f.margin, "margin", d.Margin, "distance from the image edges in pixels")
}

// anyChanged reports whether the user set at least one watermark flag.
func (f *wmFlags) anyChanged(cmd *cobra.Command) bool {
	for _, n := range wmNames {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

// override applies every flag the user explicitly set on top of base.
func (f *wmFlags) override(cmd *cobra.Command, base domain.WatermarkConfig) domain.WatermarkConfig {
	ch := cmd.Flags().Changed
	if ch("image") {
		base.Image = f.image
	}
	if ch("position") {
		base.Position = f.position
	}
	if ch("opacity") {
		base.Opacity = f.opacity
	}
	if ch("scale") {
		base.Scale = f.scale
	}
	if ch("margin") {
		base.Margin = f.margin
	}
	return base
}

// formatFlag selects table or JSON output.
func addFormatFlag(cmd *cobra.Command, dst *string) {
	cmd.Flags().StringVar(dst, "format", "table", "output format: table or json")
}

// runBatch runs a processing use case and prints its report.
func runBatch(cmd *cobra.Command, e *env, format string, run func(*app.Processor) (*domain.Report, error)) error {
	ctx := cmd.Context()
	format, err := domain.ParseOutputFormat(format)
	if err != nil {
		return err
	}
	eng, err := e.Engine(ctx)
	if err != nil {
		return err
	}
	proc := &app.Processor{Engine: eng, Log: e.log}
	if format == "table" {
		proc.Progress = func(r domain.Result) { output.ResultLine(cmd.OutOrStdout(), r) }
	}
	report, err := run(proc)
	if report == nil {
		return err
	}
	if report.Total == 0 && err == nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "No supported image files found.")
	}
	if perr := output.Report(cmd.OutOrStdout(), report, format); perr != nil {
		return perr
	}
	if err != nil {
		return err
	}
	if report.Failed > 0 {
		return fmt.Errorf("%d file(s) failed", report.Failed)
	}
	return nil
}
