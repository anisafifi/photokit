// Package imageio wraps image decoding and encoding.
package imageio

import (
	"fmt"
	"image"
	"image/color"
	"os"

	"github.com/disintegration/imaging"

	"photokit/internal/domain"
)

// Decode opens an image and applies its EXIF orientation so that pixels are
// upright (placement is calculated on the displayed image).
func Decode(path string) (image.Image, error) {
	return imaging.Open(path, imaging.AutoOrientation(true))
}

// DecodeRaw opens an image without orientation handling (used for signatures).
func DecodeRaw(path string) (image.Image, error) {
	return imaging.Open(path)
}

// Encode writes img to path in the given format. quality only applies to JPEG.
// Transparent images are flattened onto white when encoded as JPEG.
func Encode(path string, img image.Image, f domain.Format, quality int) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	var encErr error
	switch f {
	case domain.FormatJPEG:
		encErr = imaging.Encode(out, flatten(img), imaging.JPEG, imaging.JPEGQuality(quality))
	case domain.FormatPNG:
		encErr = imaging.Encode(out, img, imaging.PNG)
	default:
		encErr = fmt.Errorf("%w: cannot encode %s", domain.ErrUnsupportedFormat, f)
	}
	if cerr := out.Close(); encErr == nil {
		encErr = cerr
	}
	return encErr
}

func flatten(img image.Image) image.Image {
	if o, ok := img.(interface{ Opaque() bool }); ok && o.Opaque() {
		return img
	}
	b := img.Bounds()
	bg := imaging.New(b.Dx(), b.Dy(), color.White)
	return imaging.Overlay(bg, img, image.Pt(0, 0), 1)
}
