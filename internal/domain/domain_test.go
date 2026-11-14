package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFormatFromPath(t *testing.T) {
	cases := map[string]Format{
		"a.JPG": FormatJPEG, "a.jpeg": FormatJPEG, "x/y.png": FormatPNG, "a.CR2": FormatRAW,
		"a.dng": FormatRAW, "a.tif": FormatTIFF, "a.txt": FormatUnknown, "noext": FormatUnknown,
	}
	for in, want := range cases {
		if got := FormatFromPath(in); got != want {
			t.Errorf("FormatFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderCapability(t *testing.T) {
	if !FormatJPEG.CanRender() || !FormatPNG.CanRender() {
		t.Error("jpeg and png must be renderable")
	}
	for _, f := range []Format{FormatRAW, FormatTIFF, FormatHEIC, FormatWebP, FormatUnknown} {
		if f.CanRender() {
			t.Errorf("%s must not be renderable", f)
		}
	}
	if !FormatRAW.CanEditMetadata() || FormatUnknown.CanEditMetadata() {
		t.Error("raw metadata editable, unknown not")
	}
}

func TestParsePosition(t *testing.T) {
	for _, p := range Positions {
		got, err := ParsePosition(string(p))
		if err != nil || got != p {
			t.Errorf("ParsePosition(%q) = %q, %v", p, got, err)
		}
	}
	if got, err := ParsePosition(" Bottom_Right "); err != nil || got != BottomRight {
		t.Errorf("normalisation failed: %q %v", got, err)
	}
	if _, err := ParsePosition("middle"); err == nil || !strings.Contains(err.Error(), "bottom-right") {
		t.Errorf("expected helpful error, got %v", err)
	}
}

func TestWatermarkValidateReportsEverything(t *testing.T) {
	err := WatermarkConfig{Position: "nope", Opacity: 2, Scale: 0, Margin: -1}.Validate()
	if err == nil || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected invalid config, got %v", err)
	}
	for _, want := range []string{"image is required", "position", "opacity", "scale", "margin"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
	ok := DefaultWatermark()
	ok.Image = "sig.png"
	if err := ok.Validate(); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestMetadataOverride(t *testing.T) {
	base := Metadata{Creator: "A", Copyright: "C", Keywords: []string{"x"}}
	got := base.Override(Metadata{Creator: "B", Keywords: []string{"y", "z"}})
	want := Metadata{Creator: "B", Copyright: "C", Keywords: []string{"y", "z"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
	if !reflect.DeepEqual(base.Keywords, []string{"x"}) {
		t.Error("Override must not mutate the receiver")
	}
	if !(Metadata{}).IsEmpty() || (Metadata{Title: "t"}).IsEmpty() {
		t.Error("IsEmpty wrong")
	}
}

func TestExportFormat(t *testing.T) {
	for in, want := range map[string]ExportFormat{"": ExportOriginal, "ORIGINAL": ExportOriginal, "jpg": ExportJPEG, "jpeg": ExportJPEG, "png": ExportPNG} {
		if got, err := ParseExportFormat(in); err != nil || got != want {
			t.Errorf("ParseExportFormat(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseExportFormat("webp"); err == nil {
		t.Error("webp must be rejected")
	}
	if ExportOriginal.Target(FormatRAW) != FormatRAW || ExportJPEG.Target(FormatPNG) != FormatJPEG {
		t.Error("Target wrong")
	}
}

func TestReportCountsAndJSON(t *testing.T) {
	var r Report
	r.Add(Result{Status: StatusProcessed})
	r.Add(Result{Status: StatusSkipped})
	r.Add(Result{Status: StatusFailed})
	r.Duration = 12400 * time.Millisecond
	if r.Total != 3 || r.Processed != 1 || r.Skipped != 1 || r.Failed != 1 {
		t.Fatalf("counters wrong: %+v", r)
	}
	b, err := r.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"total":3`, `"processed":1`, `"skipped":1`, `"failed":1`, `"duration":"12.4s"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("json missing %s: %s", want, b)
		}
	}
}
