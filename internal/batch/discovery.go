// Package batch finds input files and runs work over them with a bounded
// worker pool.
package batch

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"photokit/internal/domain"
	"photokit/internal/filesystem"
)

// DiscoverOptions controls file discovery.
type DiscoverOptions struct {
	Recursive bool
	// Include/Exclude are glob patterns (path.Match syntax) matched against
	// both the file name and the slash-separated path relative to its input.
	Include []string
	Exclude []string
	// ExcludeDirs are canonical directories never entered (e.g. the output
	// directory, so exports are never processed again).
	ExcludeDirs []string
}

// Discover expands inputs (files and directories) into photos.
//
// Within directories it never follows symbolic links and ignores hidden
// entries (including PhotoKit's own temporary files). Files explicitly named
// by the caller are always considered. The second return value lists files
// that were seen but cannot be processed.
func Discover(inputs []string, opts DiscoverOptions) ([]domain.Photo, []domain.Result, error) {
	for _, p := range append(append([]string(nil), opts.Include...), opts.Exclude...) {
		if _, err := path.Match(p, ""); err != nil {
			return nil, nil, domain.Invalidf("bad pattern %q: %v", p, err)
		}
	}

	var (
		photos  []domain.Photo
		skipped []domain.Result
		seen    = map[string]bool{}
	)

	filtered := func(rel string) bool {
		slash := filepath.ToSlash(rel)
		base := path.Base(slash)
		matches := func(pats []string) bool {
			for _, p := range pats {
				if ok, _ := path.Match(p, base); ok {
					return true
				}
				if ok, _ := path.Match(p, slash); ok {
					return true
				}
			}
			return false
		}
		if len(opts.Include) > 0 && !matches(opts.Include) {
			return true
		}
		return matches(opts.Exclude)
	}

	add := func(full, rel string, explicit bool) {
		format := domain.FormatFromPath(full)
		if format == domain.FormatUnknown {
			if explicit {
				skipped = append(skipped, domain.Result{
					Source: full, Status: domain.StatusSkipped,
					Reason: "unsupported file type " + fmt.Sprintf("%q", filepath.Ext(full)),
				})
			}
			return
		}
		if filtered(rel) {
			return
		}
		key := full
		if c, err := filesystem.Canonical(full); err == nil {
			key = c
		}
		if seen[key] {
			return
		}
		seen[key] = true
		photos = append(photos, domain.Photo{Path: full, Rel: rel, Format: format})
	}

	for _, in := range inputs {
		info, err := os.Stat(in) // explicit inputs may be symlinks
		if err != nil {
			return nil, nil, fmt.Errorf("input %q: %w", in, err)
		}
		if !info.IsDir() {
			add(in, filepath.Base(in), true)
			continue
		}
		canonRoot, err := filesystem.Canonical(in)
		if err != nil {
			return nil, nil, err
		}
		err = filepath.WalkDir(in, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == in {
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 || strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(in, p)
			if err != nil {
				return err
			}
			if d.IsDir() {
				if !opts.Recursive {
					return fs.SkipDir
				}
				canon := filepath.Join(canonRoot, rel)
				for _, ex := range opts.ExcludeDirs {
					if filesystem.IsWithin(ex, canon) {
						return fs.SkipDir
					}
				}
				return nil
			}
			add(p, rel, false)
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("scanning %q: %w", in, err)
		}
	}

	sort.SliceStable(photos, func(i, j int) bool { return photos[i].Path < photos[j].Path })
	return photos, skipped, nil
}
