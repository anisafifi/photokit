package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"photokit/internal/domain"
	"photokit/internal/filesystem"
)

// Job is one photo and where its output goes.
type Job struct {
	Photo domain.Photo
	Dest  string
}

// ExecOptions control output handling.
type ExecOptions struct {
	// Overwrite allows replacing an existing output file.
	Overwrite bool
	// InPlace allows the destination to be the source itself. Only metadata
	// edits requested explicitly by the user use this.
	InPlace bool
}

// Execute runs the pipeline for one job and never panics on bad input: every
// outcome, including failure, is returned as a Result.
func (p *Pipeline) Execute(ctx context.Context, job Job, opt ExecOptions) domain.Result {
	res := domain.Result{Source: job.Photo.Path, Dest: job.Dest}
	fail := func(err error) domain.Result {
		res.Status = domain.StatusFailed
		res.Error = err.Error()
		return res
	}
	skip := func(reason string) domain.Result {
		res.Status = domain.StatusSkipped
		res.Reason = reason
		return res
	}

	if err := ctx.Err(); err != nil {
		return skip("cancelled")
	}
	ops, err := p.Plan(job.Photo)
	if err != nil {
		if isUnsupported(err) {
			return skip(err.Error())
		}
		return fail(err)
	}
	for _, op := range ops {
		res.Operations = append(res.Operations, op.Name())
	}
	if len(res.Operations) == 0 {
		res.Operations = []string{"copy"}
	}

	if !opt.InPlace && filesystem.SameFile(job.Photo.Path, job.Dest) {
		return fail(domain.ErrSourceOverwrite)
	}
	if !opt.InPlace && !opt.Overwrite && filesystem.Exists(job.Dest) {
		return skip(domain.ErrOutputExists.Error())
	}
	if err := filesystem.EnsureDir(filepath.Dir(job.Dest)); err != nil {
		return fail(err)
	}

	st := &State{Photo: job.Photo, Dest: job.Dest, Target: p.Target(job.Photo), Engine: p.Engine}
	defer st.cleanup()

	for _, op := range ops {
		if err := op.Apply(ctx, st); err != nil {
			if errors.Is(err, context.Canceled) {
				return skip("cancelled")
			}
			return fail(fmt.Errorf("%s: %w", op.Name(), st.redact(err)))
		}
	}
	work, err := st.EnsureWork() // plain export: copy the original
	if err != nil {
		return fail(err)
	}
	_ = filesystem.PreserveAttrs(job.Photo.Path, work) // best effort: keep mode and mtime
	if err := filesystem.Commit(work, job.Dest, opt.Overwrite || opt.InPlace); err != nil {
		if errors.Is(err, domain.ErrOutputExists) {
			return skip(err.Error())
		}
		return fail(err)
	}
	res.Status = domain.StatusProcessed
	return res
}
