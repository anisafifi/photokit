package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	writableOnce sync.Once
	writable     map[string]string // lower-case name -> canonical spelling
	writableErr  error
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
	if err := checkUpdate("writing metadata", file, so, se, err); err != nil {
		return err
	}
	if len(md.Tags) > 0 || len(md.Remove) > 0 {
		// The user named these tags explicitly, so "this file type can't hold
		// that tag" must not pass silently.
		if w := tagWarning(se); w != "" {
			return fmt.Errorf("exiftool writing metadata %s: %s", filepath.Base(file), w)
		}
	}
	return nil
}

func tagWarning(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		l := strings.TrimSpace(line)
		if !strings.HasPrefix(l, "Warning:") {
			continue
		}
		low := strings.ToLower(l)
		for _, marker := range []string{"isn't writable", "doesn't exist", "is not defined", "can't ", "invalid date", "not in printconv", "not a valid"} {
			if strings.Contains(low, marker) {
				return strings.TrimSpace(strings.TrimPrefix(l, "Warning:"))
			}
		}
	}
	return ""
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
	args := append([]string{"-j", "-G1", "-a"}, baseArgs...)
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

func firstFloat(m map[string]any, parse func(string) (float64, bool), keys ...string) *float64 {
	for _, k := range keys {
		if v, ok := parse(asString(m[k])); ok {
			return &v
		}
	}
	return nil
}

func firstRating(m map[string]any, keys ...string) *int {
	for _, k := range keys {
		if s := asString(m[k]); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v >= -1 && v <= 5 {
				return &v
			}
		}
	}
	return nil
}

func toMetadata(m map[string]any) domain.Metadata {
	return domain.Metadata{
		Creator:      firstString(m, readCreator...),
		Copyright:    firstString(m, readCopyright...),
		Rights:       firstString(m, readRights...),
		Title:        firstString(m, readTitle...),
		Description:  firstString(m, readDescription...),
		Keywords:     firstList(m, readKeywords...),
		Credit:       firstString(m, readCredit...),
		Source:       firstString(m, readSource...),
		Instructions: firstString(m, readInstructions...),
		Headline:     firstString(m, readHeadline...),
		Writer:       firstString(m, readWriter...),
		Owner:        firstString(m, readOwner...),
		WebStatement: firstString(m, readWebStatement...),
		Label:        firstString(m, readLabel...),
		Rating:       firstRating(m, readRating...),
		Location:     firstString(m, readLocation...),
		City:         firstString(m, readCity...),
		State:        firstString(m, readState...),
		Country:      firstString(m, readCountry...),
		CountryCode:  firstString(m, readCountryCode...),
		Latitude:     firstFloat(m, parseCoordinate, readLatitude...),
		Longitude:    firstFloat(m, parseCoordinate, readLongitude...),
		Altitude:     firstFloat(m, parseAltitude, readAltitude...),
		DateTaken:    firstString(m, readDateTaken...),
		Contact: domain.Contact{
			Email:      firstString(m, readContactEmail...),
			Phone:      firstString(m, readContactPhone...),
			URL:        firstString(m, readContactURL...),
			Address:    firstString(m, readContactAddr...),
			City:       firstString(m, readContactCity...),
			Region:     firstString(m, readContactRegion...),
			PostalCode: firstString(m, readContactPostal...),
			Country:    firstString(m, readContactCtry...),
		},
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
func (e *ExifTool) Inspect(ctx context.Context, path string, opts InspectOptions) (*domain.Info, error) {
	m, err := e.tags(ctx, path)
	if err != nil {
		return nil, err
	}
	camera := strings.TrimSpace(firstString(m, "IFD0:Make") + " " + firstString(m, "IFD0:Model"))
	if mk, md := firstString(m, "IFD0:Make"), firstString(m, "IFD0:Model"); mk != "" && strings.HasPrefix(md, mk) {
		camera = md // models such as "Canon EOS R5" already include the make
	}
	info := &domain.Info{
		Path:      path,
		Format:    domain.FormatFromPath(path).String(),
		Width:     firstInt(m, "File:ImageWidth", "PNG:ImageWidth", "ExifIFD:ExifImageWidth", "IFD0:ImageWidth"),
		Height:    firstInt(m, "File:ImageHeight", "PNG:ImageHeight", "ExifIFD:ExifImageHeight", "IFD0:ImageHeight"),
		Camera:    camera,
		Lens:      firstString(m, "ExifIFD:LensModel", "Composite:LensID", "ExifIFD:LensInfo"),
		DateTaken: firstString(m, "ExifIFD:DateTimeOriginal", "XMP-exif:DateTimeOriginal", "ExifIFD:CreateDate", "IFD0:ModifyDate"),
		Metadata:  toMetadata(m),
	}
	if opts.All || len(opts.Tags) > 0 {
		info.Tags = SelectTags(m, opts.Tags)
	}
	return info, nil
}

// SelectTags returns the tags of m matching patterns (all tags when there
// are none). Patterns are "Tag" or "Group:Tag", case-insensitive, with * and ?
// wildcards; a trailing "#" is ignored. The SourceFile key is never included.
func SelectTags(m map[string]any, patterns []string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k == "SourceFile" {
			continue
		}
		if len(patterns) == 0 || matchesAny(k, patterns) {
			out[k] = v
		}
	}
	return out
}

func matchesAny(key string, patterns []string) bool {
	lk := strings.ToLower(key)
	tagPart := lk
	if i := strings.LastIndex(lk, ":"); i >= 0 {
		tagPart = lk[i+1:]
	}
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(p), "#"))
		target := tagPart
		if strings.Contains(p, ":") {
			target = lk
		}
		if ok, _ := path.Match(p, target); ok {
			return true
		}
	}
	return false
}

// writableSet loads (once) the names of every tag ExifTool can write.
func (e *ExifTool) writableSet(ctx context.Context) (map[string]string, error) {
	e.writableOnce.Do(func() {
		so, se, err := e.run(ctx, "-listw")
		if err != nil {
			e.writableErr = fmt.Errorf("exiftool -listw: %v: %s", err, firstLine(se))
			return
		}
		set := map[string]string{}
		for i, line := range strings.Split(so, "\n") {
			if i == 0 { // "Writable tags:"
				continue
			}
			for _, name := range strings.Fields(line) {
				set[strings.ToLower(name)] = name
			}
		}
		if len(set) == 0 {
			e.writableErr = fmt.Errorf("exiftool -listw returned no tags")
			return
		}
		e.writable = set
	})
	return e.writable, e.writableErr
}

// CheckTags implements Engine.
func (e *ExifTool) CheckTags(ctx context.Context, md domain.Metadata) error {
	type item struct{ name, kind string }
	var items []item
	for _, n := range md.SortedTagNames() {
		items = append(items, item{n, "tags"})
	}
	for _, n := range md.Remove {
		items = append(items, item{n, "remove"})
	}
	if len(items) == 0 {
		return nil
	}
	known, err := e.writableSet(ctx)
	if err != nil {
		return err
	}
	var problems []string
	for _, it := range items {
		_, tag, _ := domain.SplitTag(it.name)
		if strings.ContainsAny(tag, "*?") || (it.kind == "remove" && strings.EqualFold(tag, "all")) {
			continue
		}
		if _, ok := known[strings.ToLower(tag)]; ok {
			continue
		}
		msg := fmt.Sprintf("metadata.%s: %q is not a tag ExifTool can write", it.kind, it.name)
		if sug := suggestTags(known, tag); len(sug) > 0 {
			msg += " (did you mean " + strings.Join(sug, ", ") + "?)"
		}
		problems = append(problems, msg)
	}
	if len(problems) == 0 {
		return nil
	}
	return domain.Invalidf("%s\nFind tag names with: photokit metadata tags FILTER", strings.Join(problems, "\n"))
}

func suggestTags(known map[string]string, tag string) []string {
	lt := strings.ToLower(tag)
	type cand struct {
		name string
		dist int
	}
	var cands []cand
	for k, canonical := range known {
		d := editDistance(lt, k)
		switch {
		case d <= 2 && len(lt) >= 4:
			cands = append(cands, cand{canonical, d})
		case len(lt) >= 4 && len(k) >= 4 && (strings.Contains(k, lt) || strings.Contains(lt, k)):
			cands = append(cands, cand{canonical, 3})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		if len(cands[i].name) != len(cands[j].name) {
			return len(cands[i].name) < len(cands[j].name)
		}
		return cands[i].name < cands[j].name
	})
	var out []string
	for _, c := range cands {
		out = append(out, c.name)
		if len(out) == 5 {
			break
		}
	}
	return out
}

// editDistance is the optimal-string-alignment distance: insertions,
// deletions, substitutions and swaps of adjacent letters (the usual typo)
// each cost 1.
func editDistance(a, b string) int {
	if d := len(a) - len(b); d > 3 || d < -3 {
		return 99
	}
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}

// ListTags implements Engine by streaming ExifTool's tag database (-listx).
func (e *ExifTool) ListTags(ctx context.Context, group string) ([]TagInfo, error) {
	cmd := exec.CommandContext(ctx, e.Path, "-listx")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	type key struct{ g, n string }
	seen := map[key]TagInfo{}
	var g0, g1 string
	dec := xml.NewDecoder(stdout)
	for {
		tok, err := dec.Token()
		if err != nil {
			break // io.EOF, or the process died: checked through Wait below
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch el.Name.Local {
		case "table":
			g0, g1 = attr(el, "g0"), attr(el, "g1")
		case "tag":
			if attr(el, "writable") != "true" || attr(el, "name") == "" {
				continue
			}
			eff := g1
			if v := attr(el, "g1"); v != "" {
				eff = v
			}
			if group != "" && !strings.EqualFold(group, eff) && !strings.EqualFold(group, g0) {
				continue
			}
			k := key{eff, attr(el, "name")}
			if _, dup := seen[k]; !dup {
				seen[k] = TagInfo{Group: eff, Name: k.n, Type: attr(el, "type")}
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("exiftool -listx: %v: %s", err, firstLine(errBuf.String()))
	}
	out := make([]TagInfo, 0, len(seen))
	for _, t := range seen {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if !strings.EqualFold(out[i].Group, out[j].Group) {
			return strings.ToLower(out[i].Group) < strings.ToLower(out[j].Group)
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
