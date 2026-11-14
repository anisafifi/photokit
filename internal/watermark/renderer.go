package watermark

import (
	"fmt"
	"image"

	"photokit/internal/domain"
	"photokit/internal/imageio"
)

// Renderer holds a loaded signature and applies it to many images.
// It is safe for concurrent use.
type Renderer struct {
	sig image.Image
	cfg domain.WatermarkConfig
}

// NewRenderer validates cfg and loads the signature image once.
func NewRenderer(cfg domain.WatermarkConfig) (*Renderer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	sig, err := imageio.DecodeRaw(cfg.Image)
	if err != nil {
		return nil, fmt.Errorf("loading signature %q: %w", cfg.Image, err)
	}
	return &Renderer{sig: sig, cfg: cfg}, nil
}

// Apply returns a watermarked copy of img.
func (r *Renderer) Apply(img image.Image) image.Image {
	return Composite(img, r.sig, r.cfg)
}
