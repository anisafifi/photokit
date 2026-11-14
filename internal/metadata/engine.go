// Package metadata defines the metadata engine interface and its ExifTool adapter.
package metadata

import (
	"context"

	"photokit/internal/domain"
)

// WriteOptions tune how metadata is written.
type WriteOptions struct {
	// AppendKeywords adds to existing keywords instead of replacing them.
	AppendKeywords bool
}

// Engine reads and writes metadata. ExifTool is the default implementation;
// the interface lets it be replaced or faked in tests.
type Engine interface {
	// Read returns the metadata fields PhotoKit manages.
	Read(ctx context.Context, path string) (*domain.Metadata, error)
	// Inspect returns managed metadata plus read-only technical information.
	Inspect(ctx context.Context, path string) (*domain.Info, error)
	// Write updates the non-empty fields of md on the file at path in place.
	Write(ctx context.Context, path string, md domain.Metadata, opts WriteOptions) error
	// Copy restores approved metadata from source onto destination in place.
	// Orientation, thumbnails and dimensions are handled deliberately.
	Copy(ctx context.Context, source, destination string) error
}
