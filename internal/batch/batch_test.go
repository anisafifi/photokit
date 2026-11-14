package batch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"photokit/internal/domain"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rels(photos []domain.Photo) []string {
	var out []string
	for _, p := range photos {
		out = append(out, filepath.ToSlash(p.Rel))
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDiscoverBasics(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a.jpg", "b.PNG", "notes.txt", "raw.cr2", ".hidden.jpg", ".photokit-123.jpg", "sub/c.jpeg", "sub/deep/d.jpg", ".git/e.jpg"} {
		touch(t, filepath.Join(root, f))
	}

	flat, _, err := Discover([]string{root}, DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := rels(flat), []string{"a.jpg", "b.PNG", "raw.cr2"}; !equal(got, want) {
		t.Errorf("non-recursive: got %v want %v", got, want)
	}

	rec, _, err := Discover([]string{root}, DiscoverOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := rels(rec), []string{"a.jpg", "b.PNG", "raw.cr2", "sub/c.jpeg", "sub/deep/d.jpg"}; !equal(got, want) {
		t.Errorf("recursive: got %v want %v", got, want)
	}
}

func TestDiscoverIncludeExclude(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a.jpg", "b.png", "skip_me.jpg", "sub/c.jpg"} {
		touch(t, filepath.Join(root, f))
	}
	got, _, err := Discover([]string{root}, DiscoverOptions{Recursive: true, Include: []string{"*.jpg"}, Exclude: []string{"skip_*", "sub/*"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.jpg"}; !equal(rels(got), want) {
		t.Errorf("got %v want %v", rels(got), want)
	}
	if _, _, err := Discover([]string{root}, DiscoverOptions{Include: []string{"[bad"}}); err == nil {
		t.Error("bad glob must be rejected")
	}
}

func TestDiscoverExplicitFilesAndErrors(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a.jpg"))
	touch(t, filepath.Join(root, "doc.txt"))

	photos, skipped, err := Discover([]string{filepath.Join(root, "a.jpg"), filepath.Join(root, "a.jpg"), filepath.Join(root, "doc.txt")}, DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 1 || photos[0].Rel != "a.jpg" {
		t.Errorf("duplicate explicit input must collapse: %v", photos)
	}
	if len(skipped) != 1 || skipped[0].Status != domain.StatusSkipped {
		t.Errorf("explicit unsupported file must be reported as skipped: %v", skipped)
	}
	if _, _, err := Discover([]string{filepath.Join(root, "missing.jpg")}, DiscoverOptions{}); err == nil {
		t.Error("missing input must fail fast")
	}
}

func TestDiscoverNeverFollowsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root, outside := t.TempDir(), t.TempDir()
	touch(t, filepath.Join(root, "real.jpg"))
	touch(t, filepath.Join(outside, "secret.jpg"))
	if err := os.Symlink(filepath.Join(outside, "secret.jpg"), filepath.Join(root, "link.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	got, _, err := Discover([]string{root}, DiscoverOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"real.jpg"}; !equal(rels(got), want) {
		t.Errorf("symlinks must be ignored inside directories: got %v", rels(got))
	}
}

func TestDiscoverExcludeDirs(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a.jpg"))
	touch(t, filepath.Join(root, "export", "a.jpg"))
	canon, _ := filepath.EvalSymlinks(root)
	got, _, err := Discover([]string{root}, DiscoverOptions{Recursive: true, ExcludeDirs: []string{filepath.Join(canon, "export")}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.jpg"}; !equal(rels(got), want) {
		t.Errorf("export dir must be skipped: got %v", rels(got))
	}
}

func TestRunIsBounded(t *testing.T) {
	const workers = 3
	var cur, peak int64
	items := make([]int, 30)
	Run(context.Background(), items, workers, func(_ context.Context, _ int, _ int) {
		n := atomic.AddInt64(&cur, 1)
		for {
			p := atomic.LoadInt64(&peak)
			if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt64(&cur, -1)
	})
	if peak > workers {
		t.Errorf("peak concurrency %d exceeds %d workers", peak, workers)
	}
	if peak < 2 {
		t.Errorf("expected real parallelism, peak was %d", peak)
	}
}

func TestRunProcessesEveryItemOnce(t *testing.T) {
	var mu sync.Mutex
	seen := map[int]int{}
	items := make([]string, 100)
	Run(context.Background(), items, 8, func(_ context.Context, i int, _ string) {
		mu.Lock()
		seen[i]++
		mu.Unlock()
	})
	if len(seen) != 100 {
		t.Fatalf("processed %d of 100", len(seen))
	}
	for i, n := range seen {
		if n != 1 {
			t.Errorf("item %d processed %d times", i, n)
		}
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var started int64
	Run(ctx, make([]int, 1000), 2, func(_ context.Context, _ int, _ int) {
		if atomic.AddInt64(&started, 1) == 5 {
			cancel()
		}
		time.Sleep(time.Millisecond)
	})
	if n := atomic.LoadInt64(&started); n >= 1000 {
		t.Errorf("cancellation ignored: %d items started", n)
	}
}

func TestRunEmptyAndBadWorkers(t *testing.T) {
	Run(context.Background(), []int{}, 4, func(context.Context, int, int) { t.Error("must not be called") })
	var n int64
	Run(context.Background(), []int{1, 2, 3}, 0, func(context.Context, int, int) { atomic.AddInt64(&n, 1) })
	if n != 3 {
		t.Errorf("workers<1 must still process everything, got %d", n)
	}
}
