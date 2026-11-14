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
