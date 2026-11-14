package app_test

import (
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"testing"
)

func execOut(name string, args ...string) (string, error) {
	b, err := exec.Command(name, args...).Output()
	return string(b), err
}

func load(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func imageSize(t *testing.T, path string) (int, int) {
	b := load(t, path).Bounds()
	return b.Dx(), b.Dy()
}

// isRedish reports whether the pixel is dominated by red (the test signature
// colour); the gradient background has blue fixed at 128, so R>200,B<80 is
// unambiguous even after JPEG compression.
func isRedish(t *testing.T, path string, x, y int) bool {
	r, g, b, _ := load(t, path).At(x, y).RGBA()
	return r>>8 > 200 && g>>8 < 80 && b>>8 < 80
}
