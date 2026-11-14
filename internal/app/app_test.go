package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"photokit/internal/app"
	"photokit/internal/domain"
	"photokit/internal/testutil"
)

type fixture struct {
	root, photos, out, sig string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	f := fixture{root: root, photos: filepath.Join(root, "photos"), out: filepath.Join(root, "export"), sig: filepath.Join(root, "sig.png")}
	testutil.WriteSignature(t, f.sig, 100, 40)
	return f
}

func wm(sig string) *domain.WatermarkConfig {
	c := domain.DefaultWatermark()
	c.Image = sig
	c.Opacity = 1
	c.Scale = 0.3
	c.Margin = 4
	return &c
}

func TestWatermarkKeepsSourcesAndMetadata(t *testing.T) {
	et := testutil.RequireExifTool(t)
	f := newFixture(t)
	a := testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 400, 300)
	b := testutil.WritePNG(t, filepath.Join(f.photos, "sub", "b.png"), 300, 400)
	testutil.Exif(t, et, a, "-Make=ACME", "-Model=ACME X1", "-LensModel=Prime 50", "-DateTimeOriginal=2026:01:02 03:04:05", "-GPSLatitude=10.5", "-GPSLatitudeRef=N", "-GPSLongitude=20.5", "-GPSLongitudeRef=E", "-Copyright=© Orig")
	testutil.Exif(t, et, b, "-Artist=PngArtist")
	ha, hb := testutil.Hash(t, a), testutil.Hash(t, b)

	p := &app.Processor{Engine: et}
	rep, err := p.Run(context.Background(), app.Spec{
		Inputs: []string{f.photos}, Recursive: true, Watermark: wm(f.sig), OutputDir: f.out, Jobs: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Processed != 2 || rep.Failed != 0 {
		t.Fatalf("report: %+v", rep.Results)
	}

	if testutil.Hash(t, a) != ha || testutil.Hash(t, b) != hb {
		t.Fatal("source files were modified")
	}

	outA := filepath.Join(f.out, "a.jpg")
	info, err := et.Inspect(context.Background(), outA)
	if err != nil {
		t.Fatal(err)
	}
	if info.Camera != "ACME X1" || info.Lens != "Prime 50" || !strings.HasPrefix(info.DateTaken, "2026:01:02") || info.Metadata.Copyright != "© Orig" {
		t.Errorf("metadata lost after watermarking: %+v", info)
	}
	if info.Width != 400 || info.Height != 300 {
		t.Errorf("size changed: %dx%d", info.Width, info.Height)
	}
	if out, err := execOut(et.Path, "-s3", "-GPSLatitude", outA); err != nil || !strings.Contains(out, "10") {
		t.Errorf("GPS lost: %q %v", out, err)
	}

	outB := filepath.Join(f.out, "sub", "b.png") // structure mirrored
	if i2, err := et.Inspect(context.Background(), outB); err != nil || i2.Metadata.Creator != "PngArtist" {
		t.Errorf("png metadata lost: %+v %v", i2, err)
	}

	// signature: 120x48 at (276,248); its opaque red block is the middle half
	if !isRedish(t, outA, 336, 272) {
		t.Error("signature not visible at bottom-right")
	}
}

func TestOrientationIsAppliedBeforePlacement(t *testing.T) {
	et := testutil.RequireExifTool(t)
	f := newFixture(t)
	// stored 400x300 but Orientation=6 => displayed 300x400 (portrait)
	a := testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 400, 300)
	testutil.Exif(t, et, a, "-Orientation#=6")

	p := &app.Processor{Engine: et}
	if _, err := p.Run(context.Background(), app.Spec{Inputs: []string{f.photos}, Watermark: wm(f.sig), OutputDir: f.out, Jobs: 1}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(f.out, "a.jpg")
	info, _ := et.Inspect(context.Background(), out)
	if info.Width != 300 || info.Height != 400 {
		t.Fatalf("output must be the upright 300x400, got %dx%d", info.Width, info.Height)
	}
	if o, _ := execOut(et.Path, "-s3", "-n", "-Orientation", out); strings.TrimSpace(o) != "1" {
		t.Errorf("orientation must be reset to 1, got %q", o)
	}
	// upright 300x400: signature 90x36 at (206,360), block centre (251,378)
	if !isRedish(t, out, 251, 378) {
		t.Error("signature must sit at the bottom-right of the displayed image")
	}
}

func TestMetadataSetOnCopiesAndInPlace(t *testing.T) {
	et := testutil.RequireExifTool(t)
	f := newFixture(t)
	a := testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 80, 60)
	h := testutil.Hash(t, a)
	p := &app.Processor{Engine: et}
	svc := &app.MetadataService{Engine: et, Processor: p}

	md := domain.Metadata{Creator: "New", Keywords: []string{"k"}}
	rep, err := svc.Set(context.Background(), app.Spec{Inputs: []string{f.photos}, Metadata: md, OutputDir: f.out, Export: app.ExportSpec{Suffix: "-c"}, Jobs: 1})
	if err != nil || rep.Processed != 1 {
		t.Fatalf("%v %+v", err, rep)
	}
	if testutil.Hash(t, a) != h {
		t.Fatal("default mode must not modify the original")
	}
	out := filepath.Join(f.out, "a-c.jpg")
	if m, err := et.Read(context.Background(), out); err != nil || m.Creator != "New" {
		t.Fatalf("copy metadata: %+v %v", m, err)
	}

	if _, err := svc.Set(context.Background(), app.Spec{Inputs: []string{a}, Metadata: md, InPlace: true, Jobs: 1}); err != nil {
		t.Fatal(err)
	}
	if testutil.Hash(t, a) == h {
		t.Fatal("--in-place must modify the original")
	}
	if m, _ := et.Read(context.Background(), a); m.Creator != "New" {
		t.Errorf("in-place not applied: %+v", m)
	}
}

func TestRawSupportsMetadataButNotRendering(t *testing.T) {
	et := testutil.RequireExifTool(t)
	f := newFixture(t)
	// A DNG-extension file is enough for the capability check (no pixels needed).
	raw := filepath.Join(f.photos, "r.dng")
	if err := os.MkdirAll(f.photos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(raw, []byte("not really a raw"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &app.Processor{Engine: et}
	rep, err := p.Run(context.Background(), app.Spec{Inputs: []string{raw}, Watermark: wm(f.sig), OutputDir: f.out, Jobs: 1})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Skipped != 1 || !strings.Contains(rep.Results[0].Reason, "not supported for raw") {
		t.Errorf("RAW must be skipped for watermarking: %+v", rep.Results)
	}
	if _, err := os.Stat(filepath.Join(f.out, "r.dng")); err == nil {
		t.Error("nothing may be written for skipped files")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	f := newFixture(t)
	testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 50, 50)
	eng := &testutil.FakeEngine{}
	rep, err := (&app.Processor{Engine: eng}).Run(context.Background(), app.Spec{Inputs: []string{f.photos}, Watermark: wm(f.sig), OutputDir: f.out, DryRun: true, Jobs: 1})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Planned != 1 || !rep.DryRun {
		t.Errorf("%+v", rep)
	}
	if _, err := os.Stat(f.out); !os.IsNotExist(err) {
		t.Error("dry run must not create the output directory")
	}
	if eng.Copies+eng.Writes != 0 {
		t.Error("dry run must not call the engine")
	}
}

func TestOverwriteRules(t *testing.T) {
	f := newFixture(t)
	testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 50, 50)
	run := func(over bool) *domain.Report {
		rep, err := (&app.Processor{Engine: &testutil.FakeEngine{}}).Run(context.Background(), app.Spec{Inputs: []string{f.photos}, Watermark: wm(f.sig), OutputDir: f.out, Overwrite: over, Jobs: 1})
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	if r := run(false); r.Processed != 1 {
		t.Fatalf("first run: %+v", r.Results)
	}
	if r := run(false); r.Skipped != 1 || !strings.Contains(r.Results[0].Reason, "already exists") {
		t.Fatalf("second run must skip: %+v", r.Results)
	}
	if r := run(true); r.Processed != 1 {
		t.Fatalf("--overwrite must replace: %+v", r.Results)
	}
}

func TestIdempotentWatermarkNotStacked(t *testing.T) {
	f := newFixture(t)
	a := testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 200, 150)
	p := &app.Processor{Engine: &testutil.FakeEngine{}}
	spec := app.Spec{Inputs: []string{f.photos}, Watermark: wm(f.sig), OutputDir: f.out, Overwrite: true, Jobs: 1}
	if _, err := p.Run(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	h1 := testutil.Hash(t, filepath.Join(f.out, "a.jpg"))
	// The export dir sits inside the input tree on the second run: it must be ignored.
	spec.Inputs, spec.Recursive = []string{f.root}, true
	spec.Inputs = []string{f.photos}
	if _, err := p.Run(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if h2 := testutil.Hash(t, filepath.Join(f.out, "a.jpg")); h1 != h2 {
		t.Error("running twice from the same source must give identical output")
	}
	_ = a
}

func TestExportDirInsideInputIsNeverReprocessed(t *testing.T) {
	f := newFixture(t)
	testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 60, 60)
	out := filepath.Join(f.photos, "export")
	p := &app.Processor{Engine: &testutil.FakeEngine{}}
	spec := app.Spec{Inputs: []string{f.photos}, Recursive: true, Watermark: wm(f.sig), OutputDir: out, Jobs: 1}
	for i := 0; i < 2; i++ {
		spec.Overwrite = true
		rep, err := p.Run(context.Background(), spec)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Total != 1 {
			t.Fatalf("run %d saw %d files; exports must be excluded", i, rep.Total)
		}
	}
}

func TestOutputEqualsInputDirIsRejected(t *testing.T) {
	f := newFixture(t)
	testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 60, 60)
	_, err := (&app.Processor{Engine: &testutil.FakeEngine{}}).Run(context.Background(), app.Spec{Inputs: []string{f.photos}, Watermark: wm(f.sig), OutputDir: f.photos, Jobs: 1})
	if err == nil || !strings.Contains(err.Error(), "input directory") {
		t.Errorf("got %v", err)
	}
}

func TestDuplicateOutputPathsDetected(t *testing.T) {
	f := newFixture(t)
	testutil.WriteJPEG(t, filepath.Join(f.photos, "x", "a.jpg"), 60, 60)
	testutil.WriteJPEG(t, filepath.Join(f.root, "other", "a.jpg"), 60, 60)
	rep, err := (&app.Processor{Engine: &testutil.FakeEngine{}}).Run(context.Background(), app.Spec{
		Inputs:    []string{filepath.Join(f.photos, "x", "a.jpg"), filepath.Join(f.root, "other", "a.jpg")},
		Watermark: wm(f.sig), OutputDir: f.out, Jobs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var dup string
	for _, r := range rep.Results {
		if r.Status == domain.StatusFailed {
			dup = r.Error
		}
	}
	if rep.Processed != 1 || rep.Failed != 1 || !strings.Contains(dup, "duplicate output path") {
		t.Errorf("duplicate not detected: %+v", rep.Results)
	}
}

func TestFailureDoesNotStopBatch(t *testing.T) {
	f := newFixture(t)
	testutil.WriteJPEG(t, filepath.Join(f.photos, "good.jpg"), 60, 60)
	if err := os.WriteFile(filepath.Join(f.photos, "bad.jpg"), []byte("this is not a jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := (&app.Processor{Engine: &testutil.FakeEngine{}}).Run(context.Background(), app.Spec{Inputs: []string{f.photos}, Watermark: wm(f.sig), OutputDir: f.out, Jobs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Processed != 1 || rep.Failed != 1 {
		t.Fatalf("%+v", rep.Results)
	}
	entries, _ := os.ReadDir(f.out)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".photokit-") || e.Name() == "bad.jpg" {
			t.Errorf("failed file left debris: %s", e.Name())
		}
	}
}

func TestInterruptedBatchLeavesNoPartialFiles(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 40; i++ {
		testutil.WriteJPEG(t, filepath.Join(f.photos, strings.Repeat("a", 1)+string(rune('a'+i%26))+string(rune('a'+i/26))+".jpg"), 60, 60)
	}
	eng := &testutil.FakeEngine{Delay: 20 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(60 * time.Millisecond); cancel() }()
	rep, err := (&app.Processor{Engine: eng}).Run(ctx, app.Spec{Inputs: []string{f.photos}, Watermark: wm(f.sig), OutputDir: f.out, Jobs: 2})
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("expected interruption error, got %v", err)
	}
	if rep == nil || rep.Total != 40 || rep.Processed >= 40 || rep.Processed+rep.Skipped+rep.Failed != 40 {
		t.Fatalf("every file must be accounted for: %+v", rep)
	}
	entries, _ := os.ReadDir(f.out)
	done := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".photokit-") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
		done++
	}
	if done != rep.Processed {
		t.Errorf("%d files on disk but %d reported processed", done, rep.Processed)
	}
}

func TestSpecValidation(t *testing.T) {
	good := app.Spec{Inputs: []string{"x"}, OutputDir: "o"}
	cases := map[string]func(*app.Spec){
		"no inputs":          func(s *app.Spec) { s.Inputs = nil },
		"no output":          func(s *app.Spec) { s.OutputDir = "" },
		"bad quality":        func(s *app.Spec) { s.Export.Quality = 101 },
		"suffix separator":   func(s *app.Spec) { s.Export.Suffix = "a/b" },
		"in-place + output":  func(s *app.Spec) { s.InPlace = true },
		"in-place + convert": func(s *app.Spec) { s.InPlace, s.OutputDir, s.Export.Format = true, "", domain.ExportPNG },
		"bad watermark":      func(s *app.Spec) { s.Watermark = &domain.WatermarkConfig{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := good
			mutate(&s)
			if err := s.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
	s := good
	if err := s.Validate(); err != nil || s.Jobs != 1 || s.Export.Quality != 92 {
		t.Errorf("defaults: %v %+v", err, s)
	}
}

func TestDestPath(t *testing.T) {
	ph := domain.Photo{Rel: filepath.Join("sub", "IMG.JPG"), Format: domain.FormatJPEG}
	if got, want := app.DestPath("out", ph, domain.FormatJPEG, "-s"), filepath.Join("out", "sub", "IMG-s.JPG"); got != want {
		t.Errorf("got %s want %s", got, want)
	}
	if got, want := app.DestPath("out", ph, domain.FormatPNG, ""), filepath.Join("out", "sub", "IMG.png"); got != want {
		t.Errorf("convert: got %s want %s", got, want)
	}
}

func TestConvertToPNG(t *testing.T) {
	et := testutil.RequireExifTool(t)
	f := newFixture(t)
	a := testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 80, 60)
	testutil.Exif(t, et, a, "-Artist=Conv")
	rep, err := (&app.Processor{Engine: et}).Run(context.Background(), app.Spec{Inputs: []string{f.photos}, Export: app.ExportSpec{Format: domain.ExportPNG}, OutputDir: f.out, Jobs: 1})
	if err != nil || rep.Processed != 1 {
		t.Fatalf("%v %+v", err, rep)
	}
	out := filepath.Join(f.out, "a.png")
	info, err := et.Inspect(context.Background(), out)
	if err != nil || info.Metadata.Creator != "Conv" {
		t.Errorf("converted file lost metadata: %+v %v", info, err)
	}
}

func TestPreview(t *testing.T) {
	f := newFixture(t)
	in := testutil.WriteJPEG(t, filepath.Join(f.photos, "a.jpg"), 2000, 1000)
	h := testutil.Hash(t, in)
	svc := &app.WatermarkService{}
	out := filepath.Join(f.root, "prev.jpg")
	if _, err := svc.Preview(context.Background(), app.PreviewRequest{Input: in, Output: out, Watermark: *wm(f.sig), MaxSize: 500}); err != nil {
		t.Fatal(err)
	}
	if testutil.Hash(t, in) != h {
		t.Error("preview modified the input")
	}
	w, hgt := imageSize(t, out)
	if w != 500 || hgt != 250 {
		t.Errorf("preview size %dx%d, want 500x250", w, hgt)
	}
	if _, err := svc.Preview(context.Background(), app.PreviewRequest{Input: in, Output: out, Watermark: *wm(f.sig)}); err == nil {
		t.Error("existing preview must not be overwritten without --overwrite")
	}
	if _, err := svc.Preview(context.Background(), app.PreviewRequest{Input: in, Output: in, Watermark: *wm(f.sig), Overwrite: true}); err == nil {
		t.Error("preview must refuse to overwrite its input")
	}
}
