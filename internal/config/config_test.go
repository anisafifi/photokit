package config

import (
	"os"
	"path/filepath"
	"testing"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
}

func TestDefaults(t *testing.T) {
	chdir(t, t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Jobs != DefaultJobs || c.Output != DefaultOutput || c.File != "" {
		t.Errorf("%+v", c)
	}
}

func TestFileThenEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	os.WriteFile(filepath.Join(dir, "photokit.yaml"), []byte("jobs: 6\noutput: ./out\nexiftool:\n  path: /x/exiftool\npreset_dirs: [a, b]\n"), 0o644)
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Jobs != 6 || c.Output != "./out" || c.ExifToolPath != "/x/exiftool" || len(c.PresetDirs) != 2 {
		t.Errorf("file values: %+v", c)
	}
	t.Setenv("PHOTOKIT_JOBS", "9")
	t.Setenv("PHOTOKIT_EXIFTOOL_PATH", "/env/exiftool")
	c, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Jobs != 9 {
		t.Errorf("env must override file: jobs=%d", c.Jobs)
	}
}

func TestInvalidValues(t *testing.T) {
	chdir(t, t.TempDir())
	for _, v := range []string{"0", "-1", "65", "abc"} {
		t.Setenv("PHOTOKIT_JOBS", v)
		if _, err := Load(""); err == nil {
			t.Errorf("jobs=%s must be rejected", v)
		}
	}
}

func TestSearchDirsAlwaysIncludesPresets(t *testing.T) {
	c := &Config{PresetDirs: []string{"extra"}}
	d := c.SearchDirs()
	if d[0] != "presets" || d[1] != "extra" {
		t.Errorf("%v", d)
	}
}
