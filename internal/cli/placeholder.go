package cli

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// writePlaceholderSignature renders a simple "Your Name" PNG with a
// transparent background so a freshly initialised project works end to end.
// It is meant to be replaced by the photographer's real signature.
func writePlaceholderSignature(path, text string) error {
	face := basicfont.Face7x13
	w := font.MeasureString(face, text).Ceil() + 4
	h := 13 + 4
	small := image.NewNRGBA(image.Rect(0, 0, w, h))

	draw1 := func(dx, dy int, c color.NRGBA) {
		d := &font.Drawer{
			Dst: small, Src: image.NewUniform(c), Face: face,
			Dot: fixed.P(2+dx, 2+dy+face.Ascent),
		}
		d.DrawString(text)
	}
	draw1(1, 1, color.NRGBA{A: 200})                         // soft shadow
	draw1(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255}) // white text

	big := imaging.Resize(small, w*8, h*8, imaging.NearestNeighbor)
	canvas := image.NewNRGBA(big.Bounds())
	draw.Draw(canvas, canvas.Bounds(), big, image.Point{}, draw.Src)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if err := png.Encode(f, canvas); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
