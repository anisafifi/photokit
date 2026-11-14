package preset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const good = `
version: 1
name: t
metadata:
  creator: "Me"
  keywords: [a, b]
watermark:
  enabled: true
  image: sig.png
  position: center
  opacity: 0.5
export:
  format: jpeg
  quality: 80
`

func TestLoadResolvesAssetsRelativeToPreset(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "assets/sig.png", "png")
	p := write(t, dir, "presets/p.yaml", strings.Replace(good, "image: sig.png", "image: ../assets/sig.png", 1))
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "presets", "..", "assets", "sig.png")
	if got.Watermark.Image != want {
		t.Errorf("image = %q, want %q", got.Watermark.Image, want)
	}
	cfg := got.WatermarkConfig()
	if cfg == nil || cfg.Position != "center" || cfg.Opacity != 0.5 || cfg.Scale != 0.12 || cfg.Margin != 32 {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if got.ExportFormat() != "jpeg" || got.QualityOrDefault() != 80 {
		t.Error("export settings wrong")
	}
}

func TestRejectsUnknownFields(t *testing.T) {
	for name, doc := range map[string]string{
		"top level": "version: 1\nbogus: true\n",
		"nested":    "version: 1\nmetadata:\n  creatr: typo\n",
		"watermark": "version: 1\nwatermark:\n  opacty: 1\n",
		"export":    "version: 1\nexport:\n  qualty: 3\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(doc)); err == nil {
				t.Fatal("unknown field must be rejected")
			}
		})
	}
}

func TestMalformedAndEmpty(t *testing.T) {
	if _, err := Parse([]byte("")); err == nil {
		t.Error("empty preset must fail")
	}
	if _, err := Parse([]byte("version: [unclosed")); err == nil {
		t.Error("broken yaml must fail")
	}
}

func TestValidateCollectsAllProblems(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "bad.yaml", `
version: 2
metadata:
  keywords: [""]
watermark:
  image: missing.png
  position: nowhere
  opacity: 3
export:
  format: gif
  quality: 500
  preserve_original: false
`)
	pr, err := LoadUnvalidated(p)
	if err != nil {
		t.Fatal(err)
	}
	err = pr.Validate()
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"version must be 1", "keywords[0]", "position", "opacity", "cannot read", "export format", "quality", "preserve_original"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestNeverAllowsOverwritingSources(t *testing.T) {
	pr, err := Parse([]byte("version: 1\nexport:\n  preserve_original: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pr.Validate(); err == nil || !strings.Contains(err.Error(), "never modify source") {
		t.Errorf("got %v", err)
	}
}

func TestWatermarkDisabled(t *testing.T) {
	pr, _ := Parse([]byte("version: 1\nwatermark:\n  enabled: false\n  image: whatever.png\n"))
	if pr.WatermarkConfig() != nil {
		t.Error("disabled watermark must yield no config")
	}
	if err := pr.Validate(); err != nil {
		t.Errorf("disabled watermark must not be validated: %v", err)
	}
	none, _ := Parse([]byte("version: 1\n"))
	if none.WatermarkConfig() != nil || none.Validate() != nil {
		t.Error("preset without watermark must be valid")
	}
}

func TestFindAndList(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "one.yaml", "version: 1\n")
	write(t, dir, "two.yml", "version: 1\n")
	write(t, dir, "readme.txt", "x")

	if got, err := Find("one", []string{dir}); err != nil || got != a {
		t.Errorf("Find by name: %q %v", got, err)
	}
	if got, err := Find(a, nil); err != nil || got != a {
		t.Errorf("Find by path: %q %v", got, err)
	}
	if _, err := Find("nope", []string{dir}); err == nil {
		t.Error("missing preset must fail")
	}
	if got := List([]string{dir, dir}); len(got) != 2 {
		t.Errorf("List = %v", got)
	}
}
