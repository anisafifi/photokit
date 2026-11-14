// Package domain contains the core types shared by every PhotoKit layer.
// It has no dependencies on external programs or frameworks.
package domain

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Format is a coarse image format classification.
type Format string

// Known formats.
const (
	FormatUnknown Format = ""
	FormatJPEG    Format = "jpeg"
	FormatPNG     Format = "png"
	FormatTIFF    Format = "tiff"
	FormatWebP    Format = "webp"
	FormatHEIC    Format = "heic"
	FormatRAW     Format = "raw"
)

var extensions = map[string]Format{
	".jpg": FormatJPEG, ".jpeg": FormatJPEG,
	".png": FormatPNG,
	".tif": FormatTIFF, ".tiff": FormatTIFF,
	".webp": FormatWebP,
	".heic": FormatHEIC, ".heif": FormatHEIC,
	".cr2": FormatRAW, ".cr3": FormatRAW, ".crw": FormatRAW,
	".nef": FormatRAW, ".nrw": FormatRAW,
	".arw": FormatRAW, ".sr2": FormatRAW, ".srf": FormatRAW,
	".dng": FormatRAW, ".orf": FormatRAW, ".raf": FormatRAW,
	".rw2": FormatRAW, ".pef": FormatRAW, ".srw": FormatRAW,
	".3fr": FormatRAW, ".erf": FormatRAW, ".iiq": FormatRAW,
}

// FormatFromPath classifies a file by its extension.
func FormatFromPath(path string) Format {
	return extensions[strings.ToLower(filepath.Ext(path))]
}

func (f Format) String() string {
	if f == FormatUnknown {
		return "unknown"
	}
	return string(f)
}

// CanEditMetadata reports whether metadata editing is supported.
func (f Format) CanEditMetadata() bool { return f != FormatUnknown }

// CanRender reports whether the image can be decoded, drawn on and
// re-encoded (visual watermarking). RAW files are never rendered.
func (f Format) CanRender() bool { return f == FormatJPEG || f == FormatPNG }

// Extension returns the canonical file extension for renderable formats.
func (f Format) Extension() string {
	switch f {
	case FormatJPEG:
		return ".jpg"
	case FormatPNG:
		return ".png"
	}
	return ""
}

// Photo is a discovered input file.
type Photo struct {
	Path   string // path of the file
	Rel    string // path relative to its input root (mirrored in the output)
	Format Format
}

// ExportFormat selects the output encoding.
type ExportFormat string

// Export formats.
const (
	ExportOriginal ExportFormat = "original"
	ExportJPEG     ExportFormat = "jpeg"
	ExportPNG      ExportFormat = "png"
)

// ParseExportFormat parses a user supplied export format.
func ParseExportFormat(s string) (ExportFormat, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "original":
		return ExportOriginal, nil
	case "jpeg", "jpg":
		return ExportJPEG, nil
	case "png":
		return ExportPNG, nil
	}
	return "", Invalidf("unknown export format %q (use original, jpeg or png)", s)
}

// Target returns the output Format for a source of format src.
func (e ExportFormat) Target(src Format) Format {
	switch e {
	case ExportJPEG:
		return FormatJPEG
	case ExportPNG:
		return FormatPNG
	}
	return src
}

// ParseOutputFormat validates a "--format" value for command output.
func ParseOutputFormat(s string) (string, error) {
	switch strings.ToLower(s) {
	case "", "table":
		return "table", nil
	case "json":
		return "json", nil
	}
	return "", fmt.Errorf("unknown output format %q (use table or json)", s)
}
