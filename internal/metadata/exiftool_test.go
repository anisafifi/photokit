package metadata_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"photokit/internal/domain"
	"photokit/internal/metadata"
	"photokit/internal/testutil"
)

func TestWriteReadRoundTrip(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()

	for name, mk := range map[string]func(testing.TB, string, int, int) string{
		"jpeg": testutil.WriteJPEG,
		"png":  testutil.WritePNG,
	} {
		t.Run(name, func(t *testing.T) {
			path := mk(t, filepath.Join(t.TempDir(), "a."+map[string]string{"jpeg": "jpg", "png": "png"}[name]), 64, 48)
			want := domain.Metadata{
				Creator:     "Zoë Photographer",
				Copyright:   "© 2026 Zoë Photographer",
				Rights:      "All rights reserved",
				Description: "A test picture",
				Title:       "Title",
				Keywords:    []string{"photography", "portrait"},
				Credit:      "Credit line",
				Source:      "Source line",
			}
			if err := et.Write(ctx, path, want, metadata.WriteOptions{}); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := et.Read(ctx, path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !reflect.DeepEqual(*got, domain.Metadata{
				Creator: want.Creator, Copyright: want.Copyright, Rights: want.Rights,
				Description: want.Description, Title: want.Title, Keywords: want.Keywords,
				Credit: want.Credit, Source: want.Source,
			}) {
				t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", *got, want)
			}
		})
	}
}

func TestKeywordsReplaceAndAppend(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	path := testutil.WriteJPEG(t, filepath.Join(t.TempDir(), "a.jpg"), 32, 32)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(et.Write(ctx, path, domain.Metadata{Keywords: []string{"a", "b"}}, metadata.WriteOptions{}))
	must(et.Write(ctx, path, domain.Metadata{Keywords: []string{"c"}}, metadata.WriteOptions{}))
	got, err := et.Read(ctx, path)
	must(err)
	if !reflect.DeepEqual(got.Keywords, []string{"c"}) {
		t.Fatalf("replace: got %v", got.Keywords)
	}
	must(et.Write(ctx, path, domain.Metadata{Keywords: []string{"d"}}, metadata.WriteOptions{AppendKeywords: true}))
	got, err = et.Read(ctx, path)
	must(err)
	if !reflect.DeepEqual(got.Keywords, []string{"c", "d"}) {
		t.Fatalf("append: got %v", got.Keywords)
	}
}

func TestCopyRestoresCameraDropsOrientation(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	dir := t.TempDir()
	src := testutil.WriteJPEG(t, filepath.Join(dir, "src.jpg"), 64, 48)
	dst := testutil.WriteJPEG(t, filepath.Join(dir, "dst.jpg"), 64, 48)
	testutil.Exif(t, et, src, "-Make=ACME", "-Model=ACME X1", "-Orientation#=6", "-Artist=Someone")

	if err := et.Copy(ctx, src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := et.Inspect(ctx, dst, metadata.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Camera != "ACME X1" {
		t.Errorf("camera = %q, want ACME X1", info.Camera)
	}
	if info.Metadata.Creator != "Someone" {
		t.Errorf("creator = %q", info.Metadata.Creator)
	}
	m, err := et.Read(ctx, dst)
	if err != nil || m == nil {
		t.Fatal(err)
	}
}

func TestInspectDimensions(t *testing.T) {
	et := testutil.RequireExifTool(t)
	path := testutil.WritePNG(t, filepath.Join(t.TempDir(), "a.png"), 40, 30)
	info, err := et.Inspect(context.Background(), path, metadata.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 40 || info.Height != 30 {
		t.Errorf("size = %dx%d, want 40x30", info.Width, info.Height)
	}
}

func TestReadMissingFile(t *testing.T) {
	et := testutil.RequireExifTool(t)
	if _, err := et.Read(context.Background(), filepath.Join(t.TempDir(), "nope.jpg")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
