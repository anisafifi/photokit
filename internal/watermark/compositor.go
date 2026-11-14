package watermark

import (
	"image"

	"github.com/disintegration/imaging"

	"photokit/internal/domain"
)

// Composite draws sig onto base according to cfg and returns a new image.
// The signature's alpha channel is preserved; cfg.Opacity scales it.
func Composite(base, sig image.Image, cfg domain.WatermarkConfig) *image.NRGBA {
	b := base.Bounds()
	s := sig.Bounds()
	w, h := SignatureSize(b.Dx(), b.Dy(), s.Dx(), s.Dy(), cfg.Scale)
	resized := imaging.Resize(sig, w, h, imaging.Lanczos)
	pos, err := domain.ParsePosition(cfg.Position)
	if err != nil {
		pos = domain.BottomRight
	}
	pt := Place(b.Dx(), b.Dy(), w, h, cfg.Margin, pos)
	return imaging.Overlay(base, resized, pt, cfg.Opacity)
}
