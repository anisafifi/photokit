package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors shared across packages.
var (
	ErrExifToolNotFound  = errors.New("exiftool not found")
	ErrExifToolVersion   = errors.New("unsupported exiftool version")
	ErrUnsupportedFormat = errors.New("unsupported format")
	ErrOutputExists      = errors.New("output already exists (use --overwrite to replace it)")
	ErrSourceOverwrite   = errors.New("refusing to overwrite the source file")
	ErrDuplicateOutput   = errors.New("duplicate output path")
	ErrInvalidConfig     = errors.New("invalid configuration")
)

// Invalidf returns an error wrapping ErrInvalidConfig.
func Invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}
