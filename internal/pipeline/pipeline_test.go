package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"photokit/internal/domain"
	"photokit/internal/metadata"
)

type failingEngine struct{ msg string }

func (f failingEngine) Read(context.Context, string) (*domain.Metadata, error) { return nil, nil }
func (f failingEngine) Inspect(context.Context, string, metadata.InspectOptions) (*domain.Info, error) {
	return nil, nil
}
func (f failingEngine) CheckTags(context.Context, domain.Metadata) error { return nil }
func (f failingEngine) ListTags(context.Context, string) ([]metadata.TagInfo, error) {
	return nil, nil
}
func (f failingEngine) Copy(context.Context, string, string) error { return nil }
func (f failingEngine) Write(_ context.Context, path string, _ domain.Metadata, _ metadata.WriteOptions) error {
	return errors.New(f.msg + " - " + path)
}

func TestErrorsNeverExposeTempFileNames(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in", "IMG_1.jpg")
	os.MkdirAll(filepath.Dir(src), 0o755)
	os.WriteFile(src, []byte("x"), 0o644)

	p := New(failingEngine{"boom"}, domain.ExportOriginal, MetadataOp{Metadata: domain.Metadata{Creator: "x"}})
	res := p.Execute(context.Background(), Job{
		Photo: domain.Photo{Path: src, Rel: "IMG_1.jpg", Format: domain.FormatJPEG},
		Dest:  filepath.Join(dir, "out", "IMG_1.jpg"),
	}, ExecOptions{})
	if res.Status != domain.StatusFailed {
		t.Fatalf("status %s", res.Status)
	}
	if strings.Contains(res.Error, ".photokit-") || !strings.Contains(res.Error, "IMG_1.jpg") {
		t.Errorf("error should name the photo, not a temp file: %s", res.Error)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "out"))
	if len(entries) != 0 {
		t.Errorf("failed run left files behind: %v", entries)
	}
	if b, _ := os.ReadFile(src); string(b) != "x" {
		t.Error("source modified")
	}
}

func TestPlanSkipsUnsupportedAndNoopOps(t *testing.T) {
	p := New(failingEngine{}, domain.ExportOriginal, MetadataOp{}, RenderOp{Export: domain.ExportOriginal})
	names, err := p.OperationNames(domain.Photo{Format: domain.FormatRAW})
	if err != nil || len(names) != 1 || names[0] != "copy" {
		t.Errorf("no-op pipeline should plan a plain copy: %v %v", names, err)
	}
	p = New(failingEngine{}, domain.ExportPNG, RenderOp{Export: domain.ExportPNG})
	if _, err := p.OperationNames(domain.Photo{Format: domain.FormatRAW}); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("RAW conversion must be unsupported: %v", err)
	}
}
