package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"photokit/internal/testutil"
)

// run executes the CLI in-process in dir and returns stdout, stderr, exit code.
func run(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	var out, errb bytes.Buffer
	code := Run(context.Background(), args, &out, &errb)
	return out.String(), errb.String(), code
}

func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, _, code := run(t, dir, "init", "--creator", "Jane Doe"); code != 0 {
		t.Fatal("init failed")
	}
	testutil.WriteJPEG(t, filepath.Join(dir, "photos", "a.jpg"), 300, 200)
	testutil.WriteJPEG(t, filepath.Join(dir, "photos", "sub", "b.jpg"), 300, 200)
	return dir
}

func TestInitCreatesWorkingProject(t *testing.T) {
	dir := t.TempDir()
	out, _, code := run(t, dir, "init", "--creator", "Jane Doe")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, f := range []string{"photokit.yaml", "presets/default.yaml", "assets/signature.png"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if o, _, code := run(t, dir, "preset", "validate", "default"); code != 0 || !strings.Contains(o, "OK") {
		t.Errorf("generated preset must validate: %s", o)
	}
	preset, _ := os.ReadFile(filepath.Join(dir, "presets", "default.yaml"))
	if !strings.Contains(string(preset), "Jane Doe") {
		t.Error("creator not written to preset")
	}

	// never overwrites
	os.WriteFile(filepath.Join(dir, "presets", "default.yaml"), []byte("mine"), 0o644)
	out, _, _ = run(t, dir, "init")
	if b, _ := os.ReadFile(filepath.Join(dir, "presets", "default.yaml")); string(b) != "mine" || !strings.Contains(out, "exists") {
		t.Error("init must not overwrite existing files")
	}
	run(t, dir, "init", "--force")
	if b, _ := os.ReadFile(filepath.Join(dir, "presets", "default.yaml")); string(b) == "mine" {
		t.Error("--force must overwrite")
	}
}

func TestProcessPresetEndToEnd(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)

	out, _, code := run(t, dir, "process", "photos", "-r", "--preset", "default", "--dry-run")
	if code != 0 || !strings.Contains(out, "Dry run: 2 file(s): 2 planned") {
		t.Fatalf("dry run: %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "export")); err == nil {
		t.Fatal("dry run created output")
	}

	out, _, code = run(t, dir, "process", "photos", "-r", "-p", "default")
	if code != 0 || !strings.Contains(out, "2 processed") {
		t.Fatalf("process: %d\n%s", code, out)
	}
	for _, f := range []string{"export/a.jpg", "export/sub/b.jpg"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}

	out, _, code = run(t, dir, "metadata", "show", "export/a.jpg", "--format", "json")
	var got []struct {
		Metadata struct{ Creator, Copyright string }
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || len(got) != 1 || got[0].Metadata.Creator != "Jane Doe" {
		t.Errorf("metadata show: %d %s", code, out)
	}
}

func TestProcessOverrides(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	out, _, code := run(t, dir, "process", "photos", "-p", "default", "--no-watermark", "--creator", "Override", "--suffix", "-x", "-o", "out2", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	var rep struct {
		Processed int
		Results   []struct {
			Operations  []string
			Destination string
		}
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Processed != 1 || strings.Join(rep.Results[0].Operations, ",") != "metadata" || !strings.HasSuffix(rep.Results[0].Destination, filepath.Join("out2", "a-x.jpg")) {
		t.Errorf("overrides not applied: %+v", rep)
	}
	o, _, _ := run(t, dir, "info", "out2/a-x.jpg")
	if !strings.Contains(o, "Override") {
		t.Errorf("--creator should override the preset:\n%s", o)
	}
}

func TestProcessRejectsBadInput(t *testing.T) {
	dir := project(t)
	cases := map[string][]string{
		"unknown preset":    {"process", "photos", "-p", "nope"},
		"missing preset":    {"process", "photos"},
		"conflicting flags": {"process", "photos", "-p", "default", "--no-watermark", "--opacity", "0.5"},
		"bad export format": {"process", "photos", "-p", "default", "--export-format", "gif"},
		"bad jobs":          {"process", "photos", "-p", "default", "--jobs", "999"},
		"missing input":     {"process", "nowhere", "-p", "default"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, errs, code := run(t, dir, args...)
			if code == 0 || !strings.Contains(errs, "Error:") {
				t.Errorf("expected failure, got code %d, stderr %q", code, errs)
			}
		})
	}
}

func TestMalformedPresetIsRejectedBeforeProcessing(t *testing.T) {
	dir := project(t)
	os.WriteFile(filepath.Join(dir, "presets", "bad.yaml"), []byte("version: 1\nwatermark:\n  opacity: 9\n  extra: 1\n"), 0o644)
	_, errs, code := run(t, dir, "process", "photos", "-p", "bad")
	if code == 0 {
		t.Fatalf("must fail, stderr: %s", errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "export")); err == nil {
		t.Error("nothing may be written for an invalid preset")
	}
	out, _, code := run(t, dir, "preset", "validate", "bad", "default")
	if code == 0 || !strings.Contains(out, "INVALID  bad") || !strings.Contains(out, "OK       presets") {
		t.Errorf("validate output:\n%s", out)
	}
}

func TestWatermarkApplyAndPreview(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	out, _, code := run(t, dir, "watermark", "apply", "photos", "--image", "assets/signature.png", "--position", "center", "-o", "wm", "--quality", "80")
	if code != 0 || !strings.Contains(out, "1 processed") {
		t.Fatalf("apply: %d\n%s", code, out)
	}
	if _, errs, code := run(t, dir, "watermark", "apply", "photos", "--image", "assets/signature.png", "--position", "diagonal"); code == 0 || !strings.Contains(errs, "position") {
		t.Errorf("bad position must fail: %s", errs)
	}
	if _, _, code := run(t, dir, "watermark", "apply", "photos"); code == 0 {
		t.Error("--image is required")
	}
	out, _, code = run(t, dir, "watermark", "preview", "photos/a.jpg", "--image", "assets/signature.png", "-o", "p.jpg")
	if code != 0 || !strings.Contains(out, "Preview written") {
		t.Errorf("preview: %d %s", code, out)
	}
}

func TestMetadataSet(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	if _, errs, code := run(t, dir, "metadata", "set", "photos"); code == 0 || !strings.Contains(errs, "nothing to set") {
		t.Errorf("empty set must fail: %s", errs)
	}
	out, _, code := run(t, dir, "metadata", "set", "photos", "-r", "--creator", "Zed", "--copyright", "© 2026 Zed", "--keywords", "a,b", "-o", "m")
	if code != 0 || !strings.Contains(out, "2 processed") {
		t.Fatalf("%d %s", code, out)
	}
	o, _, _ := run(t, dir, "metadata", "show", "m", "-r")
	if !strings.Contains(o, "Zed") || !strings.Contains(o, "a, b") {
		t.Errorf("show:\n%s", o)
	}
}

func TestInfoErrorsAndExitCodes(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	os.WriteFile(filepath.Join(dir, "photos", "broken.jpg"), []byte("garbage"), 0o644)
	out, _, code := run(t, dir, "info", "photos")
	if code == 0 || !strings.Contains(out, "error:") || !strings.Contains(out, "a.jpg") {
		t.Errorf("a broken file must be reported and set a failing exit code:\n%s", out)
	}
	if _, errs, code := run(t, dir, "info", "photos", "--format", "xml"); code == 0 || !strings.Contains(errs, "output format") {
		t.Errorf("bad --format: %s", errs)
	}
}

func TestExifToolMissingGivesGuidance(t *testing.T) {
	dir := project(t)
	_, errs, code := run(t, dir, "info", "photos/a.jpg", "--exiftool", "/definitely/not/exiftool")
	if code == 0 || !strings.Contains(errs, "exiftool not found") || !strings.Contains(errs, "exiftool.org") {
		t.Errorf("want install guidance, got %q", errs)
	}
	// commands that do not need ExifTool keep working
	if _, _, code := run(t, dir, "preset", "list"); code != 0 {
		t.Error("preset list must not require ExifTool")
	}
	out, _, code := run(t, dir, "version", "--exiftool", "/definitely/not/exiftool")
	if code != 0 || !strings.Contains(out, "NOT AVAILABLE") {
		t.Errorf("version must report and succeed: %s", out)
	}
}

func TestConfigFileAndEnv(t *testing.T) {
	dir := project(t)
	os.WriteFile(filepath.Join(dir, "photokit.yaml"), []byte("jobs: 0\n"), 0o644)
	if _, errs, code := run(t, dir, "preset", "list"); code == 0 || !strings.Contains(errs, "jobs") {
		t.Errorf("invalid config must fail loudly: %q", errs)
	}
	if _, errs, code := run(t, dir, "preset", "list", "--config", filepath.Join(dir, "missing.yaml")); code == 0 {
		t.Errorf("explicit missing config must fail: %q", errs)
	}
	os.WriteFile(filepath.Join(dir, "photokit.yaml"), []byte("jobs: 2\n"), 0o644)
	t.Setenv("PHOTOKIT_JOBS", "3")
	if _, _, code := run(t, dir, "preset", "list"); code != 0 {
		t.Error("valid config must load")
	}
}

func TestHelpAndVersionFlag(t *testing.T) {
	out, _, code := run(t, t.TempDir(), "--help")
	if code != 0 {
		t.Fatal("help failed")
	}
	for _, c := range []string{"init", "info", "metadata", "watermark", "process", "preset", "completion", "version"} {
		if !strings.Contains(out, c) {
			t.Errorf("help missing %q", c)
		}
	}
	if out, _, _ := run(t, t.TempDir(), "--version"); !strings.HasPrefix(out, "photokit ") {
		t.Errorf("--version: %q", out)
	}
	if out, _, code := run(t, t.TempDir(), "completion", "bash"); code != 0 || !strings.Contains(out, "bash completion") {
		t.Errorf("completion: %d", code)
	}
}

func showJSON(t *testing.T, dir string, args ...string) []map[string]any {
	t.Helper()
	out, errs, code := run(t, dir, append(args, "--format", "json")...)
	if code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	var v []map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return v
}

// tagsOf returns the "tags" object of an info result; an absent key means no matches.
func tagsOf(v map[string]any) map[string]any {
	m, _ := v["tags"].(map[string]any)
	return m
}

func TestMetadataSetExtendedFields(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	out, errs, code := run(t, dir, "metadata", "set", "photos/a.jpg", "-o", "m",
		"--creator", "Jane", "--headline", "Golden hour", "--writer", "Ed", "--owner", "Jane", "--label", "Green", "--rating", "5",
		"--location", "Ramna", "--city", "Dhaka", "--state", "Dhaka", "--country", "Bangladesh", "--country-code", "bd",
		"--latitude", "-33.9", "--longitude", "-18.4", "--altitude", "-30", "--date-taken", "2026-05-01T10:30:00+06:00",
		"--contact-email", "j@x.y", "--contact-phone", "+880", "--contact-url", "https://x.y", "--contact-city", "Dhaka", "--contact-postal-code", "1207",
		"--web-statement", "https://x.y/license")
	if code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	m := showJSON(t, dir, "metadata", "show", "m/a.jpg")[0]["metadata"].(map[string]any)
	for k, want := range map[string]any{
		"creator": "Jane", "headline": "Golden hour", "writer": "Ed", "owner": "Jane", "label": "Green", "rating": float64(5),
		"location": "Ramna", "city": "Dhaka", "state": "Dhaka", "country": "Bangladesh", "country_code": "BD",
		"web_statement": "https://x.y/license",
	} {
		if m[k] != want {
			t.Errorf("%s = %v, want %v", k, m[k], want)
		}
	}
	if lat, _ := m["latitude"].(float64); lat > -33.89 || lat < -33.91 {
		t.Errorf("southern latitude lost its sign: %v", m["latitude"])
	}
	if lon, _ := m["longitude"].(float64); lon > -18.39 || lon < -18.41 {
		t.Errorf("western longitude lost its sign: %v", m["longitude"])
	}
	if alt, _ := m["altitude"].(float64); alt > -29.9 || alt < -30.1 {
		t.Errorf("below-sea-level altitude lost its sign: %v", m["altitude"])
	}
	c := m["contact"].(map[string]any)
	if c["email"] != "j@x.y" || c["postal_code"] != "1207" || c["city"] != "Dhaka" {
		t.Errorf("contact: %v", c)
	}
	if o, _, _ := run(t, dir, "metadata", "show", "m/a.jpg"); !strings.Contains(o, "5 / 5") || !strings.Contains(o, "Bangladesh (BD)") || !strings.Contains(o, "-33.900000, -18.400000") {
		t.Errorf("table view:\n%s", o)
	}
}

func TestRatingZeroAndGPSZeroAreRealValues(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	if _, errs, code := run(t, dir, "metadata", "set", "photos/a.jpg", "-o", "z", "--rating", "0", "--latitude", "0", "--longitude", "0"); code != 0 {
		t.Fatal(errs)
	}
	m := showJSON(t, dir, "metadata", "show", "z/a.jpg")[0]["metadata"].(map[string]any)
	if r, ok := m["rating"]; !ok || r != float64(0) {
		t.Errorf("rating 0 must be written: %v", m)
	}
	if _, ok := m["latitude"]; !ok {
		t.Errorf("latitude 0 must be written: %v", m)
	}
}

func TestSetRawTagsRemoveAndStrip(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	if _, errs, code := run(t, dir, "metadata", "set", "photos/a.jpg", "-o", "s1", "--latitude", "10", "--longitude", "20", "--creator", "Keep",
		"--tag", "Comment=hello, world", "--tag", "XMP-dc:Subject=one", "--tag", "XMP-dc:Subject=two"); code != 0 {
		t.Fatal(errs)
	}
	tags := tagsOf(showJSON(t, dir, "info", "s1/a.jpg", "--tag", "Comment", "--tag", "XMP-dc:Subject")[0])
	if tags["File:Comment"] != "hello, world" {
		t.Errorf("a comma inside --tag must stay part of the value: %v", tags)
	}
	if subj, _ := tags["XMP-dc:Subject"].([]any); len(subj) != 2 {
		t.Errorf("repeating --tag must build a list: %v", tags)
	}

	if _, errs, code := run(t, dir, "metadata", "set", "s1/a.jpg", "-o", "s2", "--strip", "gps", "--remove", "Comment"); code != 0 {
		t.Fatal(errs)
	}
	m := showJSON(t, dir, "info", "s2/a.jpg")[0]["metadata"].(map[string]any)
	if _, has := m["latitude"]; has || m["creator"] != "Keep" {
		t.Errorf("strip gps must drop location only: %v", m)
	}
	left := tagsOf(showJSON(t, dir, "info", "s2/a.jpg", "--tag", "Comment")[0])
	if len(left) != 0 {
		t.Errorf("comment must be removed: %v", left)
	}

	if _, errs, code := run(t, dir, "metadata", "set", "s1/a.jpg", "-o", "s3", "--strip", "all", "--copyright", "© Fresh"); code != 0 {
		t.Fatal(errs)
	}
	m = showJSON(t, dir, "info", "s3/a.jpg")[0]["metadata"].(map[string]any)
	if m["copyright"] != "© Fresh" || m["creator"] != nil {
		t.Errorf("strip all keeps only what is written afterwards: %v", m)
	}

	// an empty value deletes the tag
	if _, errs, code := run(t, dir, "metadata", "set", "s1/a.jpg", "-o", "s4", "--tag", "XMP-dc:Subject="); code != 0 {
		t.Fatal(errs)
	}
	if left := tagsOf(showJSON(t, dir, "info", "s4/a.jpg", "--tag", "XMP-dc:Subject")[0]); len(left) != 0 {
		t.Errorf("--tag NAME= must delete: %v", left)
	}
}

func TestMetadataSetRejectsBadInputBeforeWriting(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	cases := map[string]struct {
		args []string
		want string
	}{
		"typo":         {[]string{"--tag", "Artsit=x"}, "did you mean Artist"},
		"unknown":      {[]string{"--tag", "Nonsense:Thing=x"}, "is not a tag ExifTool can write"},
		"csv option":   {[]string{"--tag", "csv=/etc/passwd"}, "is not a tag ExifTool can write"},
		"no equals":    {[]string{"--tag", "Comment"}, "NAME=VALUE"},
		"bad remove":   {[]string{"--remove", "x=y"}, "metadata.remove"},
		"lat only":     {[]string{"--latitude", "10"}, "together"},
		"lat range":    {[]string{"--latitude", "95", "--longitude", "0"}, "between -90 and 90"},
		"rating":       {[]string{"--rating", "6"}, "between 0 and 5"},
		"date":         {[]string{"--date-taken", "soon"}, "cannot understand date"},
		"strip":        {[]string{"--strip", "some"}, "none, gps, all"},
		"country code": {[]string{"--country-code", "Bangladesh"}, "ISO"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, errs, code := run(t, dir, append([]string{"metadata", "set", "photos", "-o", "never"}, c.args...)...)
			if code == 0 || !strings.Contains(errs, c.want) {
				t.Errorf("want failure mentioning %q; code=%d stderr=%q", c.want, code, errs)
			}
			if _, err := os.Stat(filepath.Join(dir, "never")); err == nil {
				t.Error("nothing may be written when validation fails")
			}
		})
	}
	// dry run validates exactly like a real run
	if _, errs, code := run(t, dir, "metadata", "set", "photos", "-o", "never", "--tag", "Bogus:Nope=1", "--dry-run"); code == 0 || !strings.Contains(errs, "not a tag") {
		t.Errorf("dry run must validate tags: %q", errs)
	}
}

func TestInfoAllAndTagFilter(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	run(t, dir, "metadata", "set", "photos/a.jpg", "-o", "m", "--creator", "Alice")

	out, _, code := run(t, dir, "info", "m/a.jpg", "--all")
	if code != 0 || !strings.Contains(out, "[IFD0]") || !strings.Contains(out, "Artist") || !strings.Contains(out, "[File]") || strings.Count(out, "\n") < 20 {
		t.Errorf("--all should list every group:\n%s", out)
	}
	out, _, _ = run(t, dir, "info", "m/a.jpg", "--tag", "artist", "--tag", "xmp-dc:*")
	if !strings.Contains(out, "Alice") || strings.Contains(out, "[File]") || strings.Count(out, "\n") > 6 {
		t.Errorf("--tag must filter:\n%s", out)
	}
	if out, _, _ := run(t, dir, "info", "m/a.jpg", "--tag", "NoSuchTagAnywhere"); !strings.Contains(out, "no matching tags") {
		t.Errorf("empty filter result must say so:\n%s", out)
	}
	if out, _, _ := run(t, dir, "metadata", "show", "m/a.jpg", "--tag", "Artist"); !strings.Contains(out, "[IFD0]") || strings.Contains(out, "Instructions") {
		t.Errorf("metadata show --tag:\n%s", out)
	}
	v := showJSON(t, dir, "info", "m/a.jpg", "--all")
	if tags, ok := v[0]["tags"].(map[string]any); !ok || len(tags) < 10 || tags["IFD0:Artist"] != "Alice" {
		t.Errorf("json --all: %v", v[0]["tags"])
	}
	if v := showJSON(t, dir, "info", "m/a.jpg"); v[0]["tags"] != nil {
		t.Error("tags must not be present unless requested")
	}
}

func TestMetadataTagsCommand(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := t.TempDir()
	out, _, code := run(t, dir, "metadata", "tags", "copyright")
	if code != 0 || !strings.Contains(out, "IPTC") || !strings.Contains(out, "CopyrightNotice") || !strings.Contains(out, "writable tag(s)") {
		t.Errorf("tags copyright:\n%s", out)
	}
	out, _, _ = run(t, dir, "metadata", "tags", "--group", "XMP-dc", "--format", "json")
	var tags []struct{ Group, Name string }
	if err := json.Unmarshal([]byte(out), &tags); err != nil || len(tags) < 10 {
		t.Fatalf("%v\n%.200s", err, out)
	}
	for _, tg := range tags {
		if tg.Group != "XMP-dc" {
			t.Errorf("group filter leaked %v", tg)
		}
	}
	if out, _, _ := run(t, dir, "metadata", "tags", "zzzznotatag"); !strings.Contains(out, "No matching tags") {
		t.Errorf("%s", out)
	}
}

const extPreset = `version: 1
name: ext
metadata:
  creator: Preset Person
  city: Dhaka
  rating: 3
  contact:
    email: preset@example.com
    city: Dhaka
  tags:
    Comment: from preset
    XMP-dc:Format: image/jpeg
  remove: [XMP-xmp:Label]
export:
  directory: ./export
`

func TestProcessPresetWithExtendedMetadataAndOverrides(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	os.WriteFile(filepath.Join(dir, "presets", "ext.yaml"), []byte(extPreset), 0o644)

	if out, _, code := run(t, dir, "preset", "validate", "ext"); code != 0 || !strings.Contains(out, "OK") {
		t.Fatalf("validate: %s", out)
	}
	if _, errs, code := run(t, dir, "process", "photos/a.jpg", "-p", "ext", "--no-watermark"); code != 0 {
		t.Fatal(errs)
	}
	v := showJSON(t, dir, "info", "export/a.jpg", "--tag", "Comment", "--tag", "XMP-dc:Format")[0]
	m := v["metadata"].(map[string]any)
	if m["creator"] != "Preset Person" || m["city"] != "Dhaka" || m["rating"] != float64(3) || m["contact"].(map[string]any)["email"] != "preset@example.com" {
		t.Errorf("preset metadata: %v", m)
	}
	if tags := v["tags"].(map[string]any); tags["File:Comment"] != "from preset" || tags["XMP-dc:Format"] != "image/jpeg" {
		t.Errorf("preset tags: %v", tags)
	}

	// flags override per field / merge per tag
	if _, errs, code := run(t, dir, "process", "photos/a.jpg", "-p", "ext", "--no-watermark", "-o", "o2",
		"--city", "Khulna", "--rating", "1", "--contact-phone", "123", "--tag", "Comment=from flag", "--strip", "gps"); code != 0 {
		t.Fatal(errs)
	}
	v = showJSON(t, dir, "info", "o2/a.jpg", "--tag", "Comment", "--tag", "XMP-dc:Format")[0]
	m = v["metadata"].(map[string]any)
	c := m["contact"].(map[string]any)
	if m["city"] != "Khulna" || m["rating"] != float64(1) || m["creator"] != "Preset Person" || c["phone"] != "123" || c["email"] != "preset@example.com" {
		t.Errorf("flags must override fields and keep the rest: %v", m)
	}
	if tags := v["tags"].(map[string]any); tags["File:Comment"] != "from flag" || tags["XMP-dc:Format"] != "image/jpeg" {
		t.Errorf("--tag must merge per tag: %v", tags)
	}
}

func TestPresetValidateCatchesTagTypos(t *testing.T) {
	testutil.RequireExifTool(t)
	dir := project(t)
	os.WriteFile(filepath.Join(dir, "presets", "typo.yaml"), []byte("version: 1\nmetadata:\n  tags:\n    Artsit: x\n  remove: [Comment, NopeNope]\n"), 0o644)
	out, _, code := run(t, dir, "preset", "validate", "typo")
	if code == 0 || !strings.Contains(out, "INVALID") || !strings.Contains(out, "Artsit") || !strings.Contains(out, "NopeNope") {
		t.Errorf("validate must report unknown tags (exit %d):\n%s", code, out)
	}
	// and the run itself refuses before writing anything
	if _, errs, code := run(t, dir, "process", "photos", "-p", "typo", "--no-watermark"); code == 0 {
		t.Errorf("process must fail on a bad tag: %s", errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "export")); err == nil {
		t.Error("nothing may be written")
	}
}

func TestPresetShowRoundTrips(t *testing.T) {
	dir := project(t)
	os.WriteFile(filepath.Join(dir, "presets", "ext.yaml"), []byte(extPreset), 0o644)
	out, _, code := run(t, dir, "preset", "show", "ext")
	if code != 0 || !strings.Contains(out, "email: preset@example.com") || !strings.Contains(out, "Comment: from preset") {
		t.Fatalf("show:\n%s", out)
	}
}
