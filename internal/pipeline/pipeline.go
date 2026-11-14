// Package pipeline runs an ordered list of operations on one photo and
// publishes the result atomically. Direct commands and presets both build
// pipelines, so they can never behave differently.
package pipeline

import (
	"errors"
	"fmt"

	"photokit/internal/domain"
	"photokit/internal/metadata"
)

// Pipeline is an ordered set of operations plus the export target.
type Pipeline struct {
	Engine metadata.Engine
	Export domain.ExportFormat
	Ops    []Operation
}

// New builds a pipeline. Export defaults to the original format.
func New(engine metadata.Engine, export domain.ExportFormat, ops ...Operation) *Pipeline {
	if export == "" {
		export = domain.ExportOriginal
	}
	return &Pipeline{Engine: engine, Export: export, Ops: ops}
}

// Target returns the format the output of photo will have.
func (p *Pipeline) Target(photo domain.Photo) domain.Format {
	return p.Export.Target(photo.Format)
}

// Plan returns the operations that will actually run for photo, or an error
// wrapping domain.ErrUnsupportedFormat when one of them cannot handle it.
func (p *Pipeline) Plan(photo domain.Photo) ([]Operation, error) {
	var ops []Operation
	for _, op := range p.Ops {
		if !op.Applies(photo) {
			continue
		}
		if err := op.Supports(photo); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

// OperationNames lists the names of the operations Plan returns, for reports.
func (p *Pipeline) OperationNames(photo domain.Photo) ([]string, error) {
	ops, err := p.Plan(photo)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ops))
	for _, op := range ops {
		names = append(names, op.Name())
	}
	if len(names) == 0 {
		names = append(names, "copy")
	}
	return names, nil
}

func isUnsupported(err error) bool { return errors.Is(err, domain.ErrUnsupportedFormat) }

func unsupported(photo domain.Photo, what string) error {
	return fmt.Errorf("%w: %s is not supported for %s files", domain.ErrUnsupportedFormat, what, photo.Format)
}
