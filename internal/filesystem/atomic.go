package filesystem

import (
	"os"
	"path/filepath"

	"photokit/internal/domain"
)

// CreateTemp creates an empty hidden temporary file in dir (the final
// destination directory, so the closing rename is atomic) and returns its path.
func CreateTemp(dir, ext string) (string, error) {
	f, err := os.CreateTemp(dir, ".photokit-*"+ext)
	if err != nil {
		return "", err
	}
	name := f.Name()
	return name, f.Close()
}

// Commit atomically moves tmp to dest. Unless overwrite is set, an existing
// destination is never replaced.
func Commit(tmp, dest string, overwrite bool) error {
	if !overwrite && Exists(dest) {
		return domain.ErrOutputExists
	}
	return os.Rename(tmp, dest)
}

// PreserveAttrs copies permission bits and modification time from src to dst.
func PreserveAttrs(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

// Remove deletes path, ignoring errors (used for temp-file cleanup).
func Remove(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

// EnsureDir creates dir and its parents.
func EnsureDir(dir string) error {
	return os.MkdirAll(filepath.Clean(dir), 0o755)
}
