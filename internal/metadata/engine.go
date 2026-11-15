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

// InspectOptions select how much Inspect returns.
type InspectOptions struct {
	// All returns every tag ExifTool finds in Info.Tags.
	All bool
	// Tags limits Info.Tags to tags matching these patterns (implies All).
	// A pattern is "Tag" or "Group:Tag", case-insensitive, with * and ? wildcards.
	Tags []string
}

// TagInfo describes one writable tag.
type TagInfo struct {
	Group string `json:"group"`
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
}

// Engine reads and writes metadata. ExifTool is the default implementation;
// the interface lets it be replaced or faked in tests.
type Engine interface {
	// Read returns the metadata fields PhotoKit manages.
	Read(ctx context.Context, path string) (*domain.Metadata, error)
	// Inspect returns managed metadata plus read-only technical information,
	// and optionally every tag (see InspectOptions).
	Inspect(ctx context.Context, path string, opts InspectOptions) (*domain.Info, error)
	// Write applies md to the file at path in place: it strips, removes and
	// sets whatever md describes.
	Write(ctx context.Context, path string, md domain.Metadata, opts WriteOptions) error
	// Copy restores approved metadata from source onto destination in place.
	// Orientation, thumbnails and dimensions are handled deliberately.
	Copy(ctx context.Context, source, destination string) error
	// CheckTags verifies, before any file is touched, that every raw tag in
	// md exists and is writable.
	CheckTags(ctx context.Context, md domain.Metadata) error
	// ListTags lists writable tags, optionally limited to a group such as
	// "XMP", "IPTC", "EXIF" or "XMP-dc".
	ListTags(ctx context.Context, group string) ([]TagInfo, error)
}
