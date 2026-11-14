package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"

	"photokit/internal/domain"
	"photokit/internal/filesystem"
	"photokit/internal/imageio"
	"photokit/internal/watermark"
)

// WatermarkService implements the signature use cases.
type WatermarkService struct {
	Processor *Processor
}

// Apply draws the signature on copies of the inputs and restores the
// originals' metadata.
func (s *WatermarkService) Apply(ctx context.Context, spec Spec) (*domain.Report, error) {
	if spec.Watermark == nil {
		return nil, domain.Invalidf("a watermark image is required")
	}
	return s.Processor.Run(ctx, spec)
}

// PreviewRequest describes a single-image preview render.
type PreviewRequest struct {
	Input     string
	Output    string // file to write; extension selects .jpg or .png
	Watermark domain.WatermarkConfig
	MaxSize   int // longest edge in pixels; 0 keeps the full size
	Overwrite bool
}

// Preview renders one image with the signature for quick visual checking. It
// never touches the input and writes no metadata.
func (s *WatermarkService) Preview(_ context.Context, req PreviewRequest) (string, error) {
	format := domain.FormatFromPath(req.Input)
	if !format.CanRender() {
		return "", fmt.Errorf("%w: previews need a JPEG or PNG, got %s", domain.ErrUnsupportedFormat, format)
	}
	out := req.Output
	if out == "" {
		out = "preview" + format.Extension()
	}
	outFormat := domain.FormatFromPath(out)
	if !outFormat.CanRender() {
		return "", domain.Invalidf("preview output must end in .jpg, .jpeg or .png (got %q)", filepath.Ext(out))
	}
	if filesystem.SameFile(req.Input, out) {
		return "", domain.ErrSourceOverwrite
	}
	if !req.Overwrite && filesystem.Exists(out) {
		return "", fmt.Errorf("%s: %w", out, domain.ErrOutputExists)
	}

	r, err := watermark.NewRenderer(req.Watermark)
	if err != nil {
		return "", err
	}
	img, err := imageio.Decode(req.Input)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", req.Input, err)
	}
	result := r.Apply(img)
	if req.MaxSize > 0 {
		b := result.Bounds()
		if b.Dx() > req.MaxSize || b.Dy() > req.MaxSize {
			img = imaging.Fit(result, req.MaxSize, req.MaxSize, imaging.Lanczos)
		} else {
			img = result
		}
	} else {
		img = result
	}

	if err := filesystem.EnsureDir(filepath.Dir(out)); err != nil {
		return "", err
	}
	tmp, err := filesystem.CreateTemp(filepath.Dir(out), filepath.Ext(out))
	if err != nil {
		return "", err
	}
	defer filesystem.Remove(tmp)
	if err := imageio.Encode(tmp, img, outFormat, 90); err != nil {
		return "", err
	}
	if err := filesystem.Commit(tmp, out, req.Overwrite); err != nil {
		return "", err
	}
	_ = os.Chmod(out, 0o644)
	return strings.TrimPrefix(out, "./"), nil
}
