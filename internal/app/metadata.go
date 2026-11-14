package app

import (
	"context"
	"fmt"

	"photokit/internal/batch"
	"photokit/internal/domain"
	"photokit/internal/metadata"
)

// MetadataService implements the metadata use cases.
type MetadataService struct {
	Engine    metadata.Engine
	Processor *Processor
}

// Inspect reads metadata for every discovered file. Per-file failures are
// reported in Info.Error rather than aborting the run.
func (s *MetadataService) Inspect(ctx context.Context, inputs []string, opts batch.DiscoverOptions, jobs int) ([]domain.Info, error) {
	if len(inputs) == 0 {
		return nil, domain.Invalidf("no input files or directories given")
	}
	photos, skipped, err := batch.Discover(inputs, opts)
	if err != nil {
		return nil, err
	}
	infos := make([]domain.Info, len(photos))
	batch.Run(ctx, photos, jobs, func(ctx context.Context, i int, ph domain.Photo) {
		info, err := s.Engine.Inspect(ctx, ph.Path)
		if err != nil {
			infos[i] = domain.Info{Path: ph.Path, Format: ph.Format.String(), Error: err.Error()}
			return
		}
		infos[i] = *info
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, sk := range skipped {
		infos = append(infos, domain.Info{Path: sk.Source, Error: sk.Reason})
	}
	return infos, nil
}

// Set writes metadata onto copies of the inputs (or in place when the spec
// says so explicitly).
func (s *MetadataService) Set(ctx context.Context, spec Spec) (*domain.Report, error) {
	if spec.Metadata.IsEmpty() {
		return nil, domain.Invalidf("nothing to set: give at least one of --creator, --copyright, --rights, --description, --title, --keywords, --credit, --source, --instructions")
	}
	if spec.Watermark != nil {
		return nil, fmt.Errorf("metadata set does not draw watermarks")
	}
	return s.Processor.Run(ctx, spec)
}
