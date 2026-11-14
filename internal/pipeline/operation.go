package pipeline

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"photokit/internal/domain"
	"photokit/internal/filesystem"
	"photokit/internal/imageio"
	"photokit/internal/metadata"
	"photokit/internal/watermark"
)

// Operation is one step of a pipeline.
type Operation interface {
	Name() string
	// Applies reports whether the step does anything for photo.
	Applies(photo domain.Photo) bool
	// Supports returns an error wrapping ErrUnsupportedFormat if the step
	// cannot handle photo. Only called when Applies is true.
	Supports(photo domain.Photo) error
	// Apply performs the step on the state's working file.
	Apply(ctx context.Context, st *State) error
}

// State is the per-photo working state shared by operations. All work
// happens on temporary files next to the destination; the source is only
// ever read.
type State struct {
	Photo  domain.Photo
	Dest   string
	Target domain.Format
	Engine metadata.Engine

	work  string
	temps []string
}

// NewTemp creates a temporary file in the destination directory.
func (s *State) NewTemp() (string, error) {
	tmp, err := filesystem.CreateTemp(filepath.Dir(s.Dest), filepath.Ext(s.Dest))
	if err != nil {
		return "", err
	}
	s.temps = append(s.temps, tmp)
	return tmp, nil
}

// Work returns the current working file ("" before any operation ran).
func (s *State) Work() string { return s.work }

// EnsureWork guarantees a working file exists, copying the source when no
// earlier step produced one.
func (s *State) EnsureWork() (string, error) {
	if s.work != "" {
		return s.work, nil
	}
	tmp, err := s.NewTemp()
	if err != nil {
		return "", err
	}
	if err := filesystem.CopyFile(s.Photo.Path, tmp); err != nil {
		return "", err
	}
	s.work = tmp
	return tmp, nil
}

// SetWork makes path the working file and discards the previous one.
func (s *State) SetWork(path string) {
	if s.work != "" && s.work != path {
		filesystem.Remove(s.work)
	}
	s.work = path
}

// redact replaces internal temporary file names in err with the name of the
// photo being processed, so users never see PhotoKit's scratch files.
func (s *State) redact(err error) error {
	msg := err.Error()
	name := filepath.Base(s.Photo.Path)
	for _, t := range s.temps {
		msg = strings.ReplaceAll(msg, t, s.Photo.Path)
		msg = strings.ReplaceAll(msg, filepath.Base(t), name)
	}
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

func (s *State) cleanup() {
	for _, t := range s.temps {
		filesystem.Remove(t) // no-op for the one that was renamed into place
	}
}

// MetadataOp writes metadata onto the working file.
type MetadataOp struct {
	Metadata domain.Metadata
	Options  metadata.WriteOptions
}

// Name implements Operation.
func (MetadataOp) Name() string { return "metadata" }

// Applies implements Operation.
func (o MetadataOp) Applies(domain.Photo) bool { return !o.Metadata.IsEmpty() }

// Supports implements Operation.
func (MetadataOp) Supports(p domain.Photo) error {
	if !p.Format.CanEditMetadata() {
		return unsupported(p, "metadata editing")
	}
	return nil
}

// Apply implements Operation.
func (o MetadataOp) Apply(ctx context.Context, st *State) error {
	work, err := st.EnsureWork()
	if err != nil {
		return err
	}
	return st.Engine.Write(ctx, work, o.Metadata, o.Options)
}

// RenderOp decodes the original, optionally draws a signature, re-encodes it
// in the target format and restores approved metadata from the original.
type RenderOp struct {
	Renderer *watermark.Renderer // nil: convert only
	Export   domain.ExportFormat
	Quality  int
}

// Name implements Operation.
func (o RenderOp) Name() string {
	if o.Renderer != nil {
		return "watermark"
	}
	return "convert"
}

func (o RenderOp) needed(p domain.Photo) bool {
	return o.Renderer != nil || o.Export.Target(p.Format) != p.Format
}

// Applies implements Operation.
func (o RenderOp) Applies(p domain.Photo) bool { return o.needed(p) }

// Supports implements Operation. RAW files are never rendered.
func (o RenderOp) Supports(p domain.Photo) error {
	if !p.Format.CanRender() {
		return unsupported(p, "image rendering ("+o.Name()+")")
	}
	return nil
}

// Apply implements Operation.
func (o RenderOp) Apply(ctx context.Context, st *State) error {
	img, err := imageio.Decode(st.Photo.Path) // always from the original
	if err != nil {
		return err
	}
	if o.Renderer != nil {
		img = o.Renderer.Apply(img)
	}
	tmp, err := st.NewTemp()
	if err != nil {
		return err
	}
	if err := imageio.Encode(tmp, img, st.Target, o.Quality); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := st.Engine.Copy(ctx, st.Photo.Path, tmp); err != nil {
		return err
	}
	st.SetWork(tmp)
	return nil
}
