package cli

import (
	"strings"

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
	cmd.AddCommand(newMetadataShowCmd(e), newMetadataSetCmd(e), newMetadataTagsCmd(e))
	return cmd
}

func newMetadataShowCmd(e *env) *cobra.Command {
	var (
		in     inputFlags
		rd     readFlags
		format string
		jobs   int
	)
	cmd := &cobra.Command{
		Use:   "show <file|dir>...",
		Short: "Show the copyright, descriptive and location metadata",
		Long: `Show the metadata PhotoKit manages: rights, descriptions, keywords, location,
GPS, rating, capture date and contact details. Fields that are empty are left out
(the core rights fields are always shown).

Use --all to list every tag ExifTool finds, or --tag to pick specific ones.`,
		Example: `  photokit metadata show photo.jpg
  photokit metadata show ./export --recursive --format json
  photokit metadata show photo.jpg --tag "*GPS*" --tag Rating`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			infos, err := inspect(cmd, e, args, in, &jobs, format, rd.options())
			if err != nil {
				return err
			}
			return finishInspect(cmd, infos, format, output.Metadata)
		},
	}
	in.add(cmd)
	rd.add(cmd)
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
		Long: `Write metadata to every photo. Named fields (--creator, --city, --latitude, ...)
are written consistently to EXIF, IPTC and XMP so the standards never disagree.
For anything else ExifTool supports, use --tag NAME=VALUE; --remove deletes tags
and --strip removes location data or everything. Order of operations: strip,
remove, named fields, then --tag (so --tag wins over a named field).

By default PhotoKit leaves the originals untouched and writes modified copies
into the output directory (default ./export). Use --in-place to edit the
originals directly (this is the only command that can do so, and only when you
ask for it explicitly).

Only the fields you pass are changed; --keywords replaces the existing
keywords unless --append-keywords is given. Tag names are checked against
ExifTool before any file is touched (list them with 'photokit metadata tags').`,
		Example: `  photokit metadata set ./photos --recursive \
    --creator "Your Name" --copyright "© 2026 Your Name" \
    --rights "All rights reserved" --keywords photography --keywords portrait

  photokit metadata set ./photos --in-place --creator "Your Name"

  # location, GPS and capture date
  photokit metadata set ./trip -r --city Dhaka --country Bangladesh --country-code BD \
    --latitude 23.8103 --longitude 90.4125 --altitude 12 --date-taken "2026-05-01 10:30:00"

  # rating, label and photographer contact details
  photokit metadata set ./picks --rating 5 --label Green \
    --contact-email me@example.com --contact-url https://example.com

  # any ExifTool tag, deletions and privacy
  photokit metadata set ./photos --tag "XMP-dc:Format=image/jpeg" --tag "Comment=Shot on film"
  photokit metadata set ./photos --remove Comment --remove "XMP-dc:*"
  photokit metadata set ./photos --strip gps -o ./safe-to-publish`,
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
			meta, err := md.value(cmd)
			if err != nil {
				return err
			}
			spec := app.Spec{
				Inputs: args, Recursive: in.recursive, Include: in.include, Exclude: in.exclude,
				Metadata: meta, AppendKeywords: md.appendKeywords,
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

func newMetadataTagsCmd(e *env) *cobra.Command {
	var (
		group  string
		format string
	)
	cmd := &cobra.Command{
		Use:   "tags [filter]",
		Short: "List the tags ExifTool can write (for --tag, --remove and presets)",
		Long: `List every tag ExifTool can write, with its group, so you know what to pass to
--tag, --remove or the preset 'tags:' / 'remove:' sections. The optional filter
is a case-insensitive substring of the tag name.

Groups include EXIF, IPTC, XMP (and namespaces such as XMP-dc, XMP-photoshop,
XMP-iptcCore, XMP-xmp), GPS, ExifIFD, IFD0, ICC_Profile and many maker notes.`,
		Example: `  photokit metadata tags copyright
  photokit metadata tags --group XMP-dc
  photokit metadata tags --group IPTC --format json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := domain.ParseOutputFormat(format)
			if err != nil {
				return err
			}
			eng, err := e.Engine(cmd.Context())
			if err != nil {
				return err
			}
			tags, err := eng.ListTags(cmd.Context(), group)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				needle := strings.ToLower(args[0])
				kept := tags[:0]
				for _, t := range tags {
					if strings.Contains(strings.ToLower(t.Name), needle) {
						kept = append(kept, t)
					}
				}
				tags = kept
			}
			return output.TagList(cmd.OutOrStdout(), tags, f)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "only tags of this group, e.g. EXIF, IPTC, XMP, XMP-dc, GPS")
	addFormatFlag(cmd, &format)
	return cmd
}
