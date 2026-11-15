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

const extended = `
version: 1
name: ext
metadata:
  creator: Me
  headline: H
  writer: W
  owner: O
  web_statement: https://example.com/license
  label: Green
  rating: 4
  location: Ramna
  city: Dhaka
  state: Dhaka
  country: Bangladesh
  country_code: BD
  latitude: 23.8103
  longitude: 90.4125
  altitude: -3.5
  date_taken: 2026-05-01T10:30:00+06:00
  contact:
    email: me@example.com
    phone: "+8801700000000"
    url: https://example.com
    address: 1 Main St
    city: Dhaka
    region: Dhaka
    postal_code: "1207"
    country: Bangladesh
  tags:
    Comment: hello
    XMP-dc:Subject: [a, b]
    Orientation#: 1
  remove:
    - XMP-xmp:Label
    - "GPS*"
  strip: gps
`

func TestExtendedMetadataSection(t *testing.T) {
	p, err := Parse([]byte(extended))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid extended preset rejected: %v", err)
	}
	m := p.Metadata
	if m.Rating == nil || *m.Rating != 4 || m.Latitude == nil || *m.Latitude != 23.8103 || m.Altitude == nil || *m.Altitude != -3.5 {
		t.Errorf("numbers: %+v", m)
	}
	if m.DateTaken != "2026-05-01T10:30:00+06:00" || m.Contact.PostalCode != "1207" || m.Contact.Email != "me@example.com" {
		t.Errorf("strings: %+v", m)
	}
	if len(m.Tags["XMP-dc:Subject"]) != 2 || m.Tags["Orientation#"][0] != "1" || m.Tags["Comment"][0] != "hello" {
		t.Errorf("tags: %v", m.Tags)
	}
	if len(m.Remove) != 2 || m.Strip != "gps" {
		t.Errorf("remove/strip: %v %q", m.Remove, m.Strip)
	}
}

func TestExtendedMetadataRejectsTyposAndBadValues(t *testing.T) {
	for name, doc := range map[string]string{
		"unknown contact field": "version: 1\nmetadata:\n  contact:\n    emial: x\n",
		"unknown field":         "version: 1\nmetadata:\n  latitud: 1\n",
		"tags not a map":        "version: 1\nmetadata:\n  tags: [a, b]\n",
		"tag value is a map":    "version: 1\nmetadata:\n  tags:\n    X: {a: b}\n",
		"rating not a number":   "version: 1\nmetadata:\n  rating: lots\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(doc)); err == nil {
				t.Fatal("must be rejected at parse time")
			}
		})
	}
	p, _ := Parse([]byte(`
version: 1
metadata:
  rating: 8
  latitude: 100
  longitude: 0
  date_taken: whenever
  strip: everything
  country_code: Bangladesh
  tags:
    "bad name": x
    all: y
  remove: ["a=b"]
`))
	err := p.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"rating", "latitude", "date_taken", "strip", "country_code", "metadata.tags.bad name", "metadata.tags.all", "metadata.remove[0]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestExtendedPresetRoundTripsThroughYAML(t *testing.T) {
	p, err := Parse([]byte(extended))
	if err != nil {
		t.Fatal(err)
	}
	out, err := yamlMarshal(p)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse of 'preset show' output failed: %v\n%s", err, out)
	}
	if err := again.Validate(); err != nil {
		t.Fatalf("re-parsed preset invalid: %v", err)
	}
	if *again.Metadata.Rating != 4 || again.Metadata.Tags["XMP-dc:Subject"][1] != "b" || again.Metadata.Contact.City != "Dhaka" {
		t.Errorf("round trip lost data:\n%s", out)
	}
}
