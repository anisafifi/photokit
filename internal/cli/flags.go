package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"photokit/internal/app"
	"photokit/internal/batch"
	"photokit/internal/domain"
	"photokit/internal/metadata"
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

	headline, writer, owner, webStatement, label string
	rating                                       int

	location, city, state, country, countryCode string
	latitude, longitude, altitude               float64
	dateTaken                                   string

	contactEmail, contactPhone, contactURL, contactAddress string
	contactCity, contactRegion, contactPostal, contactCtry string

	tags   []string // NAME=VALUE
	remove []string
	strip  string
}

func (f *metaFlags) add(cmd *cobra.Command, withAppend bool) {
	fl := cmd.Flags()
	fl.StringVar(&f.creator, "creator", "", "creator / photographer name")
	fl.StringVar(&f.copyright, "copyright", "", `copyright notice, e.g. "© 2026 Your Name"`)
	fl.StringVar(&f.rights, "rights", "", `usage terms, e.g. "All rights reserved"`)
	fl.StringVar(&f.webStatement, "web-statement", "", "URL of the licence / rights statement")
	fl.StringVar(&f.title, "title", "", "title")
	fl.StringVar(&f.headline, "headline", "", "headline")
	fl.StringVar(&f.description, "description", "", "caption / description")
	fl.StringVar(&f.writer, "writer", "", "caption writer")
	fl.StringSliceVar(&f.keywords, "keywords", nil, "keyword (repeat the flag or separate with commas)")
	fl.StringVar(&f.credit, "credit", "", "credit line")
	fl.StringVar(&f.source, "source", "", "source")
	fl.StringVar(&f.instructions, "instructions", "", "special instructions")
	fl.StringVar(&f.owner, "owner", "", "camera owner")
	fl.StringVar(&f.label, "label", "", "colour label, e.g. Red")
	fl.IntVar(&f.rating, "rating", 0, "star rating, 0-5")

	fl.StringVar(&f.location, "location", "", "sub-location, e.g. a venue or neighbourhood")
	fl.StringVar(&f.city, "city", "", "city")
	fl.StringVar(&f.state, "state", "", "state / province")
	fl.StringVar(&f.country, "country", "", "country name")
	fl.StringVar(&f.countryCode, "country-code", "", "ISO country code, e.g. BD")
	fl.Float64Var(&f.latitude, "latitude", 0, "GPS latitude in decimal degrees (south is negative); needs --longitude")
	fl.Float64Var(&f.longitude, "longitude", 0, "GPS longitude in decimal degrees (west is negative); needs --latitude")
	fl.Float64Var(&f.altitude, "altitude", 0, "GPS altitude in metres (below sea level is negative); needs --latitude/--longitude")
	fl.StringVar(&f.dateTaken, "date-taken", "", `capture date, e.g. "2026-05-01 10:30:00" or "2026-05-01T10:30:00+06:00"`)

	fl.StringVar(&f.contactEmail, "contact-email", "", "photographer contact: email")
	fl.StringVar(&f.contactPhone, "contact-phone", "", "photographer contact: phone")
	fl.StringVar(&f.contactURL, "contact-url", "", "photographer contact: website")
	fl.StringVar(&f.contactAddress, "contact-address", "", "photographer contact: street address")
	fl.StringVar(&f.contactCity, "contact-city", "", "photographer contact: city")
	fl.StringVar(&f.contactRegion, "contact-region", "", "photographer contact: state / region")
	fl.StringVar(&f.contactPostal, "contact-postal-code", "", "photographer contact: postal code")
	fl.StringVar(&f.contactCtry, "contact-country", "", "photographer contact: country")

	fl.StringArrayVar(&f.tags, "tag", nil, `set any ExifTool tag: NAME=VALUE, e.g. --tag "XMP-dc:Format=image/jpeg" (repeat for several values or tags; empty value deletes)`)
	fl.StringSliceVar(&f.remove, "remove", nil, `delete tags: NAME, Group:NAME or Group:all, wildcards allowed, e.g. --remove Comment --remove "XMP:all"`)
	fl.StringVar(&f.strip, "strip", "", "remove metadata first: gps (location only) or all (everything except colour profile and orientation)")
	if withAppend {
		fl.BoolVar(&f.appendKeywords, "append-keywords", false, "add --keywords to the existing keywords instead of replacing them")
	}
}

// value assembles the metadata the user asked for. Numeric flags only count
// when they were actually passed, so 0 stays a valid value.
func (f *metaFlags) value(cmd *cobra.Command) (domain.Metadata, error) {
	ch := cmd.Flags().Changed
	md := domain.Metadata{
		Creator: f.creator, Copyright: f.copyright, Rights: f.rights,
		Description: f.description, Title: f.title, Keywords: f.keywords,
		Credit: f.credit, Source: f.source, Instructions: f.instructions,
		Headline: f.headline, Writer: f.writer, Owner: f.owner, WebStatement: f.webStatement, Label: f.label,
		Location: f.location, City: f.city, State: f.state, Country: f.country, CountryCode: f.countryCode,
		DateTaken: f.dateTaken, Remove: f.remove, Strip: f.strip,
		Contact: domain.Contact{
			Email: f.contactEmail, Phone: f.contactPhone, URL: f.contactURL, Address: f.contactAddress,
			City: f.contactCity, Region: f.contactRegion, PostalCode: f.contactPostal, Country: f.contactCtry,
		},
	}
	if ch("rating") {
		v := f.rating
		md.Rating = &v
	}
	if ch("latitude") {
		v := f.latitude
		md.Latitude = &v
	}
	if ch("longitude") {
		v := f.longitude
		md.Longitude = &v
	}
	if ch("altitude") {
		v := f.altitude
		md.Altitude = &v
	}
	for _, t := range f.tags {
		name, val, ok := strings.Cut(t, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return md, domain.Invalidf("--tag %q must look like NAME=VALUE (for example --tag \"Comment=hello\")", t)
		}
		if md.Tags == nil {
			md.Tags = map[string]domain.TagValue{}
		}
		if val == "" {
			md.Tags[name] = nil // empty value deletes the tag
			continue
		}
		md.Tags[name] = append(md.Tags[name], val)
	}
	return md, nil
}

// readFlags choose how much detail the read-only commands print.
type readFlags struct {
	all  bool
	tags []string
}

func (f *readFlags) add(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.BoolVar(&f.all, "all", false, "show every tag ExifTool finds, grouped (EXIF, IPTC, XMP, MakerNotes, ...)")
	fl.StringArrayVar(&f.tags, "tag", nil, `show only matching tags: NAME or Group:NAME, wildcards allowed (e.g. --tag Artist --tag "XMP-dc:*")`)
}

func (f *readFlags) options() metadata.InspectOptions {
	return metadata.InspectOptions{All: f.all, Tags: f.tags}
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
