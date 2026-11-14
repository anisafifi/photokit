package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"photokit/internal/domain"
)

// MinVersion is the oldest ExifTool release PhotoKit supports.
const MinVersion = 12.0

// ExifTool is the Engine implementation backed by the exiftool executable.
// Commands are always executed directly (never through a shell).
type ExifTool struct {
	Path    string
	Version string
}

var _ Engine = (*ExifTool)(nil)

// Detect locates ExifTool (configured may be an explicit path, or empty to
// search PATH), validates its version and returns an adapter.
func Detect(ctx context.Context, configured string) (*ExifTool, error) {
	name := configured
	if name == "" {
		name = "exiftool"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("%w (looked for %q)\n\n%s", domain.ErrExifToolNotFound, name, InstallHelp())
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-ver").Output()
	if err != nil {
		return nil, fmt.Errorf("running %s -ver: %w", path, err)
	}
	ver := strings.TrimSpace(string(out))
	f, err := strconv.ParseFloat(ver, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot parse version %q from %s", domain.ErrExifToolVersion, ver, path)
	}
	if f < MinVersion {
		return nil, fmt.Errorf("%w: found %s, need %.0f or newer\n\n%s", domain.ErrExifToolVersion, ver, MinVersion, InstallHelp())
	}
	return &ExifTool{Path: path, Version: ver}, nil
}

// InstallHelp returns platform specific installation guidance.
func InstallHelp() string {
	var how string
	switch runtime.GOOS {
	case "darwin":
		how = "  brew install exiftool"
	case "windows":
		how = "  winget install OliverBetz.ExifTool\n  choco install exiftool\n  scoop install exiftool"
	default:
		how = "  sudo apt install libimage-exiftool-perl     (Debian/Ubuntu)\n" +
			"  sudo dnf install perl-Image-ExifTool         (Fedora)\n" +
			"  sudo pacman -S perl-image-exiftool           (Arch)"
	}
	return "PhotoKit needs ExifTool (>= 12) to read and write metadata.\nInstall it with:\n" + how +
		"\nor download it from https://exiftool.org.\n" +
		"If it is installed in a non-standard place, use --exiftool PATH or PHOTOKIT_EXIFTOOL_PATH."
}

var baseArgs = []string{"-charset", "utf8", "-charset", "filename=utf8"}

func (e *ExifTool) run(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, e.Path, args...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	return so.String(), se.String(), err
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func abs(p string) (string, error) { return filepath.Abs(p) }

// Write implements Engine.
func (e *ExifTool) Write(ctx context.Context, path string, md domain.Metadata, opts WriteOptions) error {
	args := buildWriteArgs(md, opts)
	if len(args) == 0 {
		return nil
	}
	file, err := abs(path)
	if err != nil {
		return err
	}
	af, err := os.CreateTemp("", "photokit-args-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(af.Name())
	if _, err := af.WriteString(strings.Join(args, "\n") + "\n"); err != nil {
		af.Close()
		return err
	}
	if err := af.Close(); err != nil {
		return err
	}

	cmdArgs := append([]string{"-overwrite_original", "-m"}, baseArgs...)
	cmdArgs = append(cmdArgs, "-@", af.Name(), file)
	so, se, err := e.run(ctx, cmdArgs...)
	return checkUpdate("writing metadata", file, so, se, err)
}

// Copy implements Engine.
func (e *ExifTool) Copy(ctx context.Context, source, destination string) error {
	src, err := abs(source)
	if err != nil {
		return err
	}
	dst, err := abs(destination)
	if err != nil {
		return err
	}
	cmdArgs := append([]string{"-overwrite_original", "-m"}, baseArgs...)
	cmdArgs = append(cmdArgs, buildCopyArgs(src)...)
	cmdArgs = append(cmdArgs, dst)
	so, se, err := e.run(ctx, cmdArgs...)
	return checkUpdate("restoring metadata", dst, so, se, err)
}

func checkUpdate(what, file, stdout, stderr string, runErr error) error {
	if runErr != nil {
		if ctxErr := contextCause(runErr); ctxErr != nil {
			return ctxErr
		}
		msg := firstLine(stderr)
		if msg == "" {
			msg = firstLine(stdout)
		}
		return fmt.Errorf("exiftool %s %s: %v: %s", what, filepath.Base(file), runErr, msg)
	}
	if strings.Contains(stdout, "0 image files updated") || strings.Contains(stdout, "0 image files created") {
		msg := firstLine(stderr)
		if msg == "" {
			msg = "no changes were made"
		}
		return fmt.Errorf("exiftool %s %s: %s", what, filepath.Base(file), msg)
	}
	return nil
}

func contextCause(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "signal: killed") {
		return context.Canceled
	}
	return nil
}

// tags returns every tag of a file keyed by "-G1" group qualified name.
func (e *ExifTool) tags(ctx context.Context, path string) (map[string]any, error) {
	file, err := abs(path)
	if err != nil {
		return nil, err
	}
	args := append([]string{"-j", "-G1"}, baseArgs...)
	args = append(args, file)
	so, se, err := e.run(ctx, args...)
	if err != nil && strings.TrimSpace(so) == "" {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("exiftool reading %s: %v: %s", filepath.Base(file), err, firstLine(se))
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(so), &rows); err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("exiftool reading %s: unexpected output: %s", filepath.Base(file), firstLine(se))
	}
	if msg, ok := rows[0]["ExifTool:Error"]; ok {
		return nil, fmt.Errorf("exiftool reading %s: %v", filepath.Base(file), msg)
	}
	if err := checkFileType(path, asString(rows[0]["File:FileType"])); err != nil {
		return nil, err
	}
	return rows[0], nil
}

// checkFileType rejects files whose content does not match their extension
// (ExifTool happily reports a text file named .jpg without any error).
func checkFileType(path, detected string) error {
	want := map[domain.Format]string{
		domain.FormatJPEG: "JPEG", domain.FormatPNG: "PNG", domain.FormatTIFF: "TIFF",
		domain.FormatWebP: "WEBP", domain.FormatHEIC: "HEIC",
	}[domain.FormatFromPath(path)]
	switch {
	case want != "" && detected != want:
		return fmt.Errorf("%s is not a valid %s file (content looks like %s)", filepath.Base(path), want, orUnknown(detected))
	case want == "" && (detected == "" || detected == "TXT"):
		return fmt.Errorf("%s is not a valid image file (content looks like %s)", filepath.Base(path), orUnknown(detected))
	}
	return nil
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown data"
	}
	return s
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			if s := asString(x); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func asList(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		var out []string
		for _, x := range t {
			if s := asString(x); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		if s := asString(v); s != "" {
			return []string{s}
		}
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := asString(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func firstList(m map[string]any, keys ...string) []string {
	for _, k := range keys {
		if l := asList(m[k]); len(l) > 0 {
			return l
		}
	}
	return nil
}

func firstInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if f, ok := m[k].(float64); ok && f > 0 {
			return int(f)
		}
	}
	return 0
}

func toMetadata(m map[string]any) domain.Metadata {
	return domain.Metadata{
		Creator:      firstString(m, readCreator...),
		Copyright:    firstString(m, readCopyright...),
		Rights:       firstString(m, readRights...),
		Description:  firstString(m, readDescription...),
		Title:        firstString(m, readTitle...),
		Keywords:     firstList(m, readKeywords...),
		Credit:       firstString(m, readCredit...),
		Source:       firstString(m, readSource...),
		Instructions: firstString(m, readInstructions...),
	}
}

// Read implements Engine.
func (e *ExifTool) Read(ctx context.Context, path string) (*domain.Metadata, error) {
	m, err := e.tags(ctx, path)
	if err != nil {
		return nil, err
	}
	md := toMetadata(m)
	return &md, nil
}

// Inspect implements Engine.
func (e *ExifTool) Inspect(ctx context.Context, path string) (*domain.Info, error) {
	m, err := e.tags(ctx, path)
	if err != nil {
		return nil, err
	}
	camera := strings.TrimSpace(firstString(m, "IFD0:Make") + " " + firstString(m, "IFD0:Model"))
	if mk, md := firstString(m, "IFD0:Make"), firstString(m, "IFD0:Model"); mk != "" && strings.HasPrefix(md, mk) {
		camera = md // models such as "Canon EOS R5" already include the make
	}
	return &domain.Info{
		Path:      path,
		Format:    domain.FormatFromPath(path).String(),
		Width:     firstInt(m, "File:ImageWidth", "PNG:ImageWidth", "ExifIFD:ExifImageWidth", "IFD0:ImageWidth"),
		Height:    firstInt(m, "File:ImageHeight", "PNG:ImageHeight", "ExifIFD:ExifImageHeight", "IFD0:ImageHeight"),
		Camera:    camera,
		Lens:      firstString(m, "ExifIFD:LensModel", "Composite:LensID", "ExifIFD:LensInfo"),
		DateTaken: firstString(m, "ExifIFD:DateTimeOriginal", "XMP-exif:DateTimeOriginal", "ExifIFD:CreateDate", "IFD0:ModifyDate"),
		Metadata:  toMetadata(m),
	}, nil
}
