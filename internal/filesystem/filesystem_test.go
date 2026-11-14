package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"photokit/internal/domain"
)

func write(t *testing.T, p, c string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCommitNeverReplacesUnlessAsked(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.jpg")
	write(t, dest, "old")
	tmp, err := CreateTemp(dir, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	write(t, tmp, "new")
	if err := Commit(tmp, dest, false); !errors.Is(err, domain.ErrOutputExists) {
		t.Fatalf("got %v", err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "old" {
		t.Error("existing file must be untouched")
	}
	if err := Commit(tmp, dest, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "new" {
		t.Error("overwrite must replace")
	}
	if Exists(tmp) {
		t.Error("temp file must be consumed by the rename")
	}
}

func TestTempFilesAreHiddenInDestDir(t *testing.T) {
	dir := t.TempDir()
	tmp, err := CreateTemp(dir, ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer Remove(tmp)
	if filepath.Dir(tmp) != dir || filepath.Base(tmp)[:10] != ".photokit-" || filepath.Ext(tmp) != ".png" {
		t.Errorf("unexpected temp name %s", tmp)
	}
}

func TestPreserveAttrs(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	write(t, a, "x")
	write(t, b, "y")
	mt := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	os.Chtimes(a, mt, mt)
	if err := PreserveAttrs(a, b); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(b); !st.ModTime().Equal(mt) {
		t.Errorf("mtime = %v", st.ModTime())
	}
}

func TestCanonicalAndWithin(t *testing.T) {
	dir := t.TempDir()
	canonDir, _ := filepath.EvalSymlinks(dir)
	c, err := Canonical(filepath.Join(dir, "not", "yet", "there"))
	if err != nil || c != filepath.Join(canonDir, "not", "yet", "there") {
		t.Errorf("%q %v", c, err)
	}
	if !IsWithin(canonDir, filepath.Join(canonDir, "a", "b")) || !IsWithin(canonDir, canonDir) {
		t.Error("child must be within parent")
	}
	if IsWithin(filepath.Join(canonDir, "a"), filepath.Join(canonDir, "ab")) || IsWithin(filepath.Join(canonDir, "a"), canonDir) {
		t.Error("sibling prefix / parent must not be within")
	}
}

func TestCanonicalResolvesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	os.Mkdir(real, 0o755)
	os.Symlink(real, filepath.Join(dir, "link"))
	a, _ := Canonical(filepath.Join(dir, "link", "x"))
	b, _ := Canonical(filepath.Join(real, "x"))
	if a != b {
		t.Errorf("%q != %q", a, b)
	}
}

func TestSameFile(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	write(t, a, "x")
	if !SameFile(a, filepath.Join(dir, ".", "a")) || SameFile(a, filepath.Join(dir, "missing")) {
		t.Error("SameFile wrong")
	}
}
