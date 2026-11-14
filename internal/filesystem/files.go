// Package filesystem contains small, independently testable filesystem helpers.
package filesystem

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Exists reports whether path exists (without following a final symlink).
func Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// SameFile reports whether a and b refer to the same existing file.
func SameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// CopyFile copies the contents of src to dst (created or truncated, mode 0600).
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Canonical returns an absolute path with symlinks resolved as far as the
// path exists. It also works for paths that do not exist yet.
func Canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	existing := abs
	var rest []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs, nil
		}
		rest = append([]string{filepath.Base(existing)}, rest...)
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return abs, nil
	}
	return filepath.Join(append([]string{resolved}, rest...)...), nil
}

// IsWithin reports whether child is parent or lies inside it. Both paths
// should already be canonical.
func IsWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
