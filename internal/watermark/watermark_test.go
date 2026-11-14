package watermark

import (
	"image"
	"image/color"
	"testing"

	"photokit/internal/domain"
)

func TestSignatureSize(t *testing.T) {
	cases := []struct {
		name                     string
		baseW, baseH, sigW, sigH int
		scale                    float64
		wantW, wantH             int
	}{
		{"12% of 1000 keeps aspect", 1000, 800, 400, 100, 0.12, 120, 30},
		{"square", 2000, 1000, 50, 50, 0.1, 200, 200},
		{"tall signature shrinks to fit height", 1000, 100, 100, 400, 0.5, 25, 100},
		{"tiny scale never zero", 100, 100, 50, 50, 0.001, 1, 1},
		{"scale 1 fills width", 640, 480, 640, 100, 1, 640, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, h := SignatureSize(c.baseW, c.baseH, c.sigW, c.sigH, c.scale)
			if w != c.wantW || h != c.wantH {
				t.Errorf("got %dx%d, want %dx%d", w, h, c.wantW, c.wantH)
			}
		})
	}
}

func TestPlaceNineAnchors(t *testing.T) {
	const W, H, w, h, m = 1000, 800, 100, 50, 20
	want := map[domain.Position]image.Point{
		domain.TopLeft:      {20, 20},
		domain.TopCenter:    {450, 20},
		domain.TopRight:     {880, 20},
		domain.CenterLeft:   {20, 375},
		domain.Center:       {450, 375},
		domain.CenterRight:  {880, 375},
		domain.BottomLeft:   {20, 730},
		domain.BottomCenter: {450, 730},
		domain.BottomRight:  {880, 730},
	}
	if len(want) != len(domain.Positions) {
		t.Fatal("test must cover all nine anchors")
	}
	for pos, pt := range want {
		if got := Place(W, H, w, h, m, pos); got != pt {
			t.Errorf("%s: got %v want %v", pos, got, pt)
		}
	}
}

func TestPlaceMarginNeverPushesOutside(t *testing.T) {
	// signature almost as large as the image: the margin must shrink.
	pt := Place(100, 100, 90, 90, 50, domain.BottomRight)
	if pt.X < 0 || pt.Y < 0 || pt.X+90 > 100 || pt.Y+90 > 100 {
		t.Errorf("signature escapes the image: %v", pt)
	}
}

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestCompositeOpacityAndAlpha(t *testing.T) {
	base := solid(200, 100, color.NRGBA{A: 255}) // black
	sig := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if x < 20 { // left half opaque white, right half fully transparent
				sig.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
			}
		}
	}
	cfg := domain.WatermarkConfig{Position: "top-left", Opacity: 0.5, Scale: 0.2, Margin: 0}
	out := Composite(base, sig, cfg) // signature rendered 40x40 at (0,0)

	opaque := out.NRGBAAt(5, 20)
	if opaque.R < 120 || opaque.R > 135 {
		t.Errorf("50%% white over black should be ~128, got %v", opaque)
	}
	transparent := out.NRGBAAt(35, 20)
	if transparent.R > 3 {
		t.Errorf("transparent signature pixels must leave the base untouched, got %v", transparent)
	}
	if far := out.NRGBAAt(150, 80); far != (color.NRGBA{A: 255}) {
		t.Errorf("pixels outside the signature must be unchanged, got %v", far)
	}
	// the input must not be mutated
	if base.NRGBAAt(5, 20).R != 0 {
		t.Error("base image was modified")
	}
}

func TestCompositeFullOpacityIsExact(t *testing.T) {
	base := solid(100, 100, color.NRGBA{A: 255})
	sig := solid(10, 10, color.NRGBA{R: 255, A: 255})
	out := Composite(base, sig, domain.WatermarkConfig{Position: "center", Opacity: 1, Scale: 0.1, Margin: 0})
	if got := out.NRGBAAt(50, 50); got.R != 255 {
		t.Errorf("centre pixel should be the signature colour, got %v", got)
	}
}
