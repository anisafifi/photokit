package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"photokit/internal/domain"
)

func tw(w io.Writer) *tabwriter.Writer { return tabwriter.NewWriter(w, 0, 4, 2, ' ', 0) }

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Infos prints inspection results. A single successful file gets a detailed
// vertical view, several files get one row each.
func Infos(w io.Writer, infos []domain.Info, format string) error {
	if format == "json" {
		if infos == nil {
			infos = []domain.Info{}
		}
		return JSON(w, infos)
	}
	if len(infos) == 0 {
		_, err := fmt.Fprintln(w, "No supported image files found.")
		return err
	}
	if len(infos) == 1 && infos[0].Error == "" {
		return infoDetail(w, infos[0])
	}
	t := tw(w)
	fmt.Fprintln(t, "FILE\tFORMAT\tSIZE\tCAMERA\tTAKEN\tCREATOR\tCOPYRIGHT")
	for _, i := range infos {
		if i.Error != "" {
			fmt.Fprintf(t, "%s\t\t\t\t\t\terror: %s\n", i.Path, i.Error)
			continue
		}
		size := ""
		if i.Width > 0 && i.Height > 0 {
			size = fmt.Sprintf("%dx%d", i.Width, i.Height)
		}
		fmt.Fprintf(t, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", i.Path, dash(i.Format), dash(size), dash(trunc(i.Camera, 24)),
			dash(i.DateTaken), dash(trunc(i.Metadata.Creator, 24)), dash(trunc(i.Metadata.Copyright, 32)))
	}
	return t.Flush()
}

func infoDetail(w io.Writer, i domain.Info) error {
	t := tw(w)
	row := func(k, v string) { fmt.Fprintf(t, "%s\t%s\n", k, dash(v)) }
	row("File", i.Path)
	row("Format", i.Format)
	if i.Width > 0 {
		row("Size", fmt.Sprintf("%dx%d", i.Width, i.Height))
	} else {
		row("Size", "")
	}
	row("Camera", i.Camera)
	row("Lens", i.Lens)
	row("Date taken", i.DateTaken)
	if err := t.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(w)
	t = tw(w)
	metadataRows(t, i.Metadata)
	return t.Flush()
}

func metadataRows(t *tabwriter.Writer, m domain.Metadata) {
	row := func(k, v string) { fmt.Fprintf(t, "%s\t%s\n", k, dash(v)) }
	row("Creator", m.Creator)
	row("Copyright", m.Copyright)
	row("Rights", m.Rights)
	row("Title", m.Title)
	row("Description", m.Description)
	row("Keywords", strings.Join(m.Keywords, ", "))
	row("Credit", m.Credit)
	row("Source", m.Source)
	row("Instructions", m.Instructions)
}

// Metadata prints only the managed metadata fields.
func Metadata(w io.Writer, infos []domain.Info, format string) error {
	if format == "json" {
		type entry struct {
			Path     string          `json:"path"`
			Metadata domain.Metadata `json:"metadata"`
			Error    string          `json:"error,omitempty"`
		}
		out := make([]entry, 0, len(infos))
		for _, i := range infos {
			out = append(out, entry{Path: i.Path, Metadata: i.Metadata, Error: i.Error})
		}
		return JSON(w, out)
	}
	if len(infos) == 0 {
		_, err := fmt.Fprintln(w, "No supported image files found.")
		return err
	}
	for n, i := range infos {
		if n > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, i.Path)
		if i.Error != "" {
			fmt.Fprintf(w, "  error: %s\n", i.Error)
			continue
		}
		t := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		var sb strings.Builder
		inner := tw(&sb)
		metadataRows(inner, i.Metadata)
		_ = inner.Flush()
		for _, line := range strings.Split(strings.TrimRight(sb.String(), "\n"), "\n") {
			fmt.Fprintf(t, "  %s\n", line)
		}
		_ = t.Flush()
	}
	return nil
}

// ResultLine prints one result as soon as it is known (live progress).
func ResultLine(w io.Writer, r domain.Result) {
	detail := r.Dest
	switch r.Status {
	case domain.StatusSkipped:
		detail = r.Reason
	case domain.StatusFailed:
		detail = r.Error
	}
	ops := ""
	if len(r.Operations) > 0 && r.Status != domain.StatusSkipped && r.Status != domain.StatusFailed {
		ops = " [" + strings.Join(r.Operations, "+") + "]"
	}
	fmt.Fprintf(w, "%-9s %s -> %s%s\n", r.Status, r.Source, detail, ops)
}

// Report prints the batch summary (and, for JSON, the full report).
func Report(w io.Writer, r *domain.Report, format string) error {
	if format == "json" {
		return JSON(w, r)
	}
	return summary(w, r)
}

func summary(w io.Writer, r *domain.Report) error {
	dur := r.Duration.Round(100 * time.Millisecond)
	if r.DryRun {
		_, err := fmt.Fprintf(w, "\nDry run: %d file(s): %d planned, %d skipped, %d failed (nothing was written)\n",
			r.Total, r.Planned, r.Skipped, r.Failed)
		return err
	}
	_, err := fmt.Fprintf(w, "\n%d file(s): %d processed, %d skipped, %d failed in %s\n",
		r.Total, r.Processed, r.Skipped, r.Failed, dur)
	return err
}
