package preset

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse decodes YAML, rejecting unknown fields. It does not touch the
// filesystem and does not validate values.
func Parse(data []byte) (*Preset, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var p Preset
	if err := dec.Decode(&p); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("preset is empty")
		}
		return nil, err
	}
	return &p, nil
}

// Load reads a preset file, resolves relative asset paths against the
// preset's own directory and validates it.
func Load(path string) (*Preset, error) {
	p, err := LoadUnvalidated(path)
	if err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("preset %s:\n%w", path, err)
	}
	return p, nil
}

// LoadUnvalidated parses and resolves paths but skips validation, so that
// "preset validate" can report every problem.
func LoadUnvalidated(path string) (*Preset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("preset %s: %w", path, err)
	}
	p.Path = path
	if p.Watermark != nil && p.Watermark.Image != "" && !filepath.IsAbs(p.Watermark.Image) {
		p.Watermark.Image = filepath.Join(filepath.Dir(path), p.Watermark.Image)
	}
	return p, nil
}

// Find resolves a --preset argument: an existing file path, or a bare name
// looked up as <name>.yaml / <name>.yml in dirs.
func Find(arg string, dirs []string) (string, error) {
	if st, err := os.Stat(arg); err == nil && !st.IsDir() {
		return arg, nil
	}
	if !strings.ContainsAny(arg, `/\`) {
		for _, d := range dirs {
			for _, ext := range []string{".yaml", ".yml"} {
				c := filepath.Join(d, strings.TrimSuffix(arg, filepath.Ext(arg))+ext)
				if st, err := os.Stat(c); err == nil && !st.IsDir() {
					return c, nil
				}
			}
		}
	}
	return "", fmt.Errorf("preset %q not found (searched as a path and in: %s)", arg, strings.Join(dirs, ", "))
}

// List returns the preset files (*.yaml, *.yml) found directly in dirs.
func List(dirs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if e.IsDir() || (ext != ".yaml" && ext != ".yml") {
				continue
			}
			full := filepath.Join(d, e.Name())
			if !seen[full] {
				seen[full] = true
				out = append(out, full)
			}
		}
	}
	sort.Strings(out)
	return out
}
