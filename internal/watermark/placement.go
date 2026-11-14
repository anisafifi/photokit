// Package watermark composites a transparent signature onto an image.
// It knows nothing about metadata.
package watermark

import (
	"image"
	"math"

	"photokit/internal/domain"
)

// SignatureSize computes the rendered signature size. Width is scale*baseW;
// the aspect ratio is preserved and the result is shrunk if needed to fit
// inside the base image.
func SignatureSize(baseW, baseH, sigW, sigH int, scale float64) (w, h int) {
	if sigW <= 0 || sigH <= 0 || baseW <= 0 || baseH <= 0 {
		return 1, 1
	}
	w = int(math.Round(float64(baseW) * scale))
	if w < 1 {
		w = 1
	}
	h = int(math.Round(float64(sigH) * float64(w) / float64(sigW)))
	if h < 1 {
		h = 1
	}
	if w > baseW {
		h = int(math.Round(float64(h) * float64(baseW) / float64(w)))
		w = baseW
	}
	if h > baseH {
		w = int(math.Round(float64(w) * float64(baseH) / float64(h)))
		h = baseH
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// Place returns the top-left corner for a w×h signature on a baseW×baseH
// image for the given anchor. The margin shrinks when it cannot fit.
func Place(baseW, baseH, w, h, margin int, pos domain.Position) image.Point {
	mx, my := margin, margin
	if limit := (baseW - w) / 2; mx > limit {
		mx = limit
	}
	if limit := (baseH - h) / 2; my > limit {
		my = limit
	}
	if mx < 0 {
		mx = 0
	}
	if my < 0 {
		my = 0
	}

	var x, y int
	switch pos {
	case domain.TopLeft, domain.CenterLeft, domain.BottomLeft:
		x = mx
	case domain.TopCenter, domain.Center, domain.BottomCenter:
		x = (baseW - w) / 2
	default:
		x = baseW - w - mx
	}
	switch pos {
	case domain.TopLeft, domain.TopCenter, domain.TopRight:
		y = my
	case domain.CenterLeft, domain.Center, domain.CenterRight:
		y = (baseH - h) / 2
	default:
		y = baseH - h - my
	}
	return image.Pt(x, y)
}
