// Package testutil builds fixture images in temporary directories so tests
// never depend on real photography assets.
package testutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"photokit/internal/domain"
	"photokit/internal/metadata"
)

// Gradient returns a deterministic w×h test image.
func Gradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 128, A: 255})
		}
	}
	return img
}

// WriteJPEG writes a gradient JPEG and returns its path.
func WriteJPEG(t testing.TB, path string, w, h int) string {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, Gradient(w, h), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return path
}

// WritePNG writes a gradient PNG and returns its path.
func WritePNG(t testing.TB, path string, w, h int) string {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, Gradient(w, h)); err != nil {
		t.Fatal(err)
	}
	return path
}

// WriteSignature writes a transparent PNG containing an opaque red block
// with a fully transparent border, and returns its path.
func WriteSignature(t testing.TB, path string, w, h int) string {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := h / 4; y < h*3/4; y++ {
		for x := w / 4; x < w*3/4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// Exif sets raw ExifTool assignments (e.g. "-Make=ACME") on a fixture.
func Exif(t testing.TB, et *metadata.ExifTool, path string, assignments ...string) {
	t.Helper()
	args := append([]string{"-overwrite_original", "-m"}, assignments...)
	args = append(args, path)
	if out, err := exec.Command(et.Path, args...).CombinedOutput(); err != nil {
		t.Fatalf("exiftool %v: %v\n%s", assignments, err, out)
	}
}

// RequireExifTool returns an adapter or skips the test when ExifTool is absent.
func RequireExifTool(t testing.TB) *metadata.ExifTool {
	t.Helper()
	et, err := metadata.Detect(context.Background(), "")
	if err != nil {
		t.Skipf("exiftool not available: %v", err)
	}
	return et
}

// Hash returns the SHA-256 of a file, used to prove sources stay untouched.
func Hash(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func mustMkdir(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// FakeEngine is an in-memory metadata.Engine for tests that do not need
// ExifTool. Copy and Write succeed (optionally after Delay, honouring the
// context) and are counted.
type FakeEngine struct {
	Delay time.Duration

	mu     sync.Mutex
	Writes int
	Copies int
}

var _ metadata.Engine = (*FakeEngine)(nil)

func (f *FakeEngine) wait(ctx context.Context) error {
	if f.Delay == 0 {
		return ctx.Err()
	}
	select {
	case <-time.After(f.Delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Read implements metadata.Engine.
func (f *FakeEngine) Read(context.Context, string) (*domain.Metadata, error) {
	return &domain.Metadata{}, nil
}

// Inspect implements metadata.Engine.
func (f *FakeEngine) Inspect(_ context.Context, path string) (*domain.Info, error) {
	return &domain.Info{Path: path}, nil
}

// Write implements metadata.Engine.
func (f *FakeEngine) Write(ctx context.Context, _ string, _ domain.Metadata, _ metadata.WriteOptions) error {
	if err := f.wait(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	f.Writes++
	f.mu.Unlock()
	return nil
}

// Copy implements metadata.Engine.
func (f *FakeEngine) Copy(ctx context.Context, _, _ string) error {
	if err := f.wait(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	f.Copies++
	f.mu.Unlock()
	return nil
}
