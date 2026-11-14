// Package app holds the use cases. Every command (direct or preset based)
// funnels into Processor, so behaviour is identical everywhere. This package
// does not depend on Cobra.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"photokit/internal/batch"
	"photokit/internal/domain"
	"photokit/internal/filesystem"
	"photokit/internal/metadata"
	"photokit/internal/pipeline"
	"photokit/internal/watermark"
)

// ExportSpec describes the output encoding and naming.
type ExportSpec struct {
	Format  domain.ExportFormat // original, jpeg, png
	Quality int                 // JPEG quality 1-100 (default 92)
	Suffix  string              // appended to the file name before the extension
}

// Spec is the complete description of one batch run.
type Spec struct {
	Inputs    []string
	Recursive bool
	Include   []string
	Exclude   []string

	Metadata       domain.Metadata
	AppendKeywords bool
	Watermark      *domain.WatermarkConfig
	Export         ExportSpec

	OutputDir string
	InPlace   bool // metadata edits only; modifies the originals
	Overwrite bool
	DryRun    bool
	Jobs      int
}

// Processor discovers files and runs the pipeline over them.
type Processor struct {
	Engine metadata.Engine
	Log    *slog.Logger
	// Progress, if set, is called once per file as soon as it finishes
	// (serialized; safe to print from).
	Progress func(domain.Result)
}

func (p *Processor) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Validate normalises and checks a spec.
func (s *Spec) Validate() error {
	if len(s.Inputs) == 0 {
		return domain.Invalidf("no input files or directories given")
	}
	if s.Jobs < 1 {
		s.Jobs = 1
	}
	if s.Export.Format == "" {
		s.Export.Format = domain.ExportOriginal
	}
	if s.Export.Quality == 0 {
		s.Export.Quality = 92
	}
	if s.Export.Quality < 1 || s.Export.Quality > 100 {
		return domain.Invalidf("quality must be between 1 and 100, got %d", s.Export.Quality)
	}
	if strings.ContainsAny(s.Export.Suffix, `/\`) {
		return domain.Invalidf("suffix must not contain path separators")
	}
	if s.Watermark != nil {
		if err := s.Watermark.Validate(); err != nil {
			return err
		}
	}
	if s.InPlace {
		if s.OutputDir != "" {
			return domain.Invalidf("--in-place and --output cannot be combined")
		}
		if s.Watermark != nil || s.Export.Format != domain.ExportOriginal {
			return domain.Invalidf("--in-place only supports metadata edits; rendering always writes a copy")
		}
		if s.Export.Suffix != "" {
			return domain.Invalidf("--in-place cannot be combined with a suffix")
		}
	} else if s.OutputDir == "" {
		return domain.Invalidf("an output directory is required")
	}
	return nil
}

// DestPath computes where photo's output goes.
func DestPath(outDir string, photo domain.Photo, target domain.Format, suffix string) string {
	dir, file := filepath.Split(photo.Rel)
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	if target != photo.Format {
		ext = target.Extension()
	}
	return filepath.Join(outDir, dir, base+suffix+ext)
}

func (p *Processor) buildPipeline(spec Spec) (*pipeline.Pipeline, error) {
	var ops []pipeline.Operation
	var renderer *watermark.Renderer
	if spec.Watermark != nil {
		r, err := watermark.NewRenderer(*spec.Watermark)
		if err != nil {
			return nil, err
		}
		renderer = r
	}
	// Order matters: render first (it restores metadata from the original),
	// then apply the requested metadata on top.
	if renderer != nil || spec.Export.Format != domain.ExportOriginal {
		ops = append(ops, pipeline.RenderOp{Renderer: renderer, Export: spec.Export.Format, Quality: spec.Export.Quality})
	}
	if !spec.Metadata.IsEmpty() {
		ops = append(ops, pipeline.MetadataOp{
			Metadata: spec.Metadata,
			Options:  metadata.WriteOptions{AppendKeywords: spec.AppendKeywords},
		})
	}
	return pipeline.New(p.Engine, spec.Export.Format, ops...), nil
}

func foldCase() bool { return runtime.GOOS == "windows" || runtime.GOOS == "darwin" }

// Run executes the spec and returns a report. Individual file failures do not
// stop the batch; they are recorded in the report.
func (p *Processor) Run(ctx context.Context, spec Spec) (*domain.Report, error) {
	start := time.Now()
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	pl, err := p.buildPipeline(spec)
	if err != nil {
		return nil, err
	}

	opts := batch.DiscoverOptions{Recursive: spec.Recursive, Include: spec.Include, Exclude: spec.Exclude}
	if !spec.InPlace {
		out, err := filesystem.Canonical(spec.OutputDir)
		if err != nil {
			return nil, err
		}
		for _, in := range spec.Inputs {
			canon, err := filesystem.Canonical(in)
			if err != nil {
				return nil, err
			}
			if canon == out && isDir(in) {
				return nil, domain.Invalidf("output directory %q is the input directory; exports are never mixed with sources", spec.OutputDir)
			}
		}
		opts.ExcludeDirs = []string{out} // never re-process earlier exports
	}

	photos, skipped, err := batch.Discover(spec.Inputs, opts)
	if err != nil {
		return nil, err
	}

	report := &domain.Report{DryRun: spec.DryRun}
	emit := func(r domain.Result) {
		report.Add(r)
		p.log().Debug("file", "source", r.Source, "status", r.Status, "dest", r.Dest, "reason", r.Reason, "error", r.Error)
		if p.Progress != nil {
			p.Progress(r)
		}
	}
	for _, s := range skipped {
		emit(s)
	}

	// Plan destinations and detect duplicate output paths up front.
	jobs := make([]pipeline.Job, 0, len(photos))
	claimed := map[string]string{}
	for _, ph := range photos {
		dest := ph.Path
		if !spec.InPlace {
			dest = DestPath(spec.OutputDir, ph, pl.Target(ph), spec.Export.Suffix)
		}
		key := filepath.Clean(dest)
		if foldCase() {
			key = strings.ToLower(key)
		}
		if other, dup := claimed[key]; dup {
			emit(domain.Result{
				Source: ph.Path, Dest: dest, Status: domain.StatusFailed,
				Error: fmt.Sprintf("%v: %s is also produced from %s", domain.ErrDuplicateOutput, dest, other),
			})
			continue
		}
		claimed[key] = ph.Path
		jobs = append(jobs, pipeline.Job{Photo: ph, Dest: dest})
	}

	exec := pipeline.ExecOptions{Overwrite: spec.Overwrite, InPlace: spec.InPlace}

	if spec.DryRun {
		for _, j := range jobs {
			emit(p.plan(pl, j, exec))
		}
		report.Duration = time.Since(start)
		return report, nil
	}

	if len(jobs) > 0 && !spec.InPlace {
		if err := filesystem.EnsureDir(spec.OutputDir); err != nil {
			return nil, fmt.Errorf("creating output directory: %w", err)
		}
	}

	results := make([]domain.Result, len(jobs))
	done := make([]bool, len(jobs))
	var mu sync.Mutex
	batch.Run(ctx, jobs, spec.Jobs, func(ctx context.Context, i int, j pipeline.Job) {
		r := pl.Execute(ctx, j, exec)
		mu.Lock()
		defer mu.Unlock()
		results[i], done[i] = r, true
		if p.Progress != nil {
			p.Progress(r)
		}
	})
	for i, j := range jobs {
		r := results[i]
		if !done[i] {
			r = domain.Result{Source: j.Photo.Path, Dest: j.Dest, Status: domain.StatusSkipped, Reason: "cancelled"}
		}
		report.Add(r)
		p.log().Debug("file", "source", r.Source, "status", r.Status, "dest", r.Dest, "reason", r.Reason, "error", r.Error)
	}
	report.Duration = time.Since(start)
	if err := ctx.Err(); err != nil {
		return report, fmt.Errorf("interrupted: %w", err)
	}
	return report, nil
}

// plan describes what Execute would do, without touching the filesystem.
func (p *Processor) plan(pl *pipeline.Pipeline, j pipeline.Job, opt pipeline.ExecOptions) domain.Result {
	res := domain.Result{Source: j.Photo.Path, Dest: j.Dest}
	names, err := pl.OperationNames(j.Photo)
	if err != nil {
		res.Status = domain.StatusSkipped
		res.Reason = err.Error()
		if !errors.Is(err, domain.ErrUnsupportedFormat) {
			res.Status = domain.StatusFailed
			res.Error = err.Error()
		}
		return res
	}
	res.Operations = names
	switch {
	case !opt.InPlace && filesystem.SameFile(j.Photo.Path, j.Dest):
		res.Status, res.Error = domain.StatusFailed, domain.ErrSourceOverwrite.Error()
	case !opt.InPlace && !opt.Overwrite && filesystem.Exists(j.Dest):
		res.Status, res.Reason = domain.StatusSkipped, domain.ErrOutputExists.Error()
	default:
		res.Status = domain.StatusPlanned
	}
	return res
}

func isDir(path string) bool {
	st, err := osStat(path)
	return err == nil && st.IsDir()
}
