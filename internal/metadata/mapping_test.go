package metadata

import (
	"reflect"
	"strings"
	"testing"

	"photokit/internal/domain"
)

func f64(v float64) *float64 { return &v }
func ip(v int) *int          { return &v }

func has(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func index(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

func TestEmptyMetadataProducesNoArguments(t *testing.T) {
	if args := buildWriteArgs(domain.Metadata{}, WriteOptions{}); args != nil {
		t.Errorf("got %v", args)
	}
	if args := buildWriteArgs(domain.Metadata{Strip: "none"}, WriteOptions{}); args != nil {
		t.Errorf("strip none is a no-op: %v", args)
	}
}

func TestNamedFieldsWriteEveryStandard(t *testing.T) {
	args := buildWriteArgs(domain.Metadata{Creator: "Zoë", City: "Dhaka", CountryCode: "bd", Headline: "H", Contact: domain.Contact{Email: "a@b.c"}}, WriteOptions{})
	for _, want := range []string{
		"-IFD0:Artist=Zoë", "-IPTC:By-line=Zoë", "-XMP-dc:Creator=Zoë",
		"-IPTC:City=Dhaka", "-XMP-photoshop:City=Dhaka",
		"-IPTC:Country-PrimaryLocationCode=BD", "-XMP-iptcCore:CountryCode=BD",
		"-IPTC:Headline=H", "-XMP-photoshop:Headline=H",
		"-XMP-iptcCore:CreatorWorkEmail=a@b.c", "-IPTC:CodedCharacterSet=UTF8",
	} {
		if !has(args, want) {
			t.Errorf("missing %s in %v", want, args)
		}
	}
}

func TestCopyrightMarksXMPRights(t *testing.T) {
	if !has(buildWriteArgs(domain.Metadata{Rights: "r"}, WriteOptions{}), "-XMP-xmpRights:Marked=True") {
		t.Error("rights must set Marked")
	}
	if has(buildWriteArgs(domain.Metadata{Creator: "x"}, WriteOptions{}), "-XMP-xmpRights:Marked=True") {
		t.Error("Marked must only be set with copyright or rights")
	}
}

func TestGPSUsesAbsoluteValuesAndExplicitReferences(t *testing.T) {
	cases := []struct {
		lat, lon, alt float64
		want          []string
	}{
		{23.8103, 90.4125, 12.5, []string{
			"-EXIF:GPSLatitude=23.8103", "-EXIF:GPSLatitudeRef=N", "-EXIF:GPSLongitude=90.4125", "-EXIF:GPSLongitudeRef=E",
			"-XMP-exif:GPSLatitude=23.8103", "-XMP-exif:GPSLongitude=90.4125",
			"-EXIF:GPSAltitude=12.5", "-EXIF:GPSAltitudeRef#=0", "-XMP-exif:GPSAltitudeRef#=0"}},
		{-33.9, -18.4, -30, []string{
			"-EXIF:GPSLatitude=33.9", "-EXIF:GPSLatitudeRef=S", "-EXIF:GPSLongitude=18.4", "-EXIF:GPSLongitudeRef=W",
			"-XMP-exif:GPSLatitude=-33.9", "-XMP-exif:GPSLongitude=-18.4",
			"-EXIF:GPSAltitude=30", "-EXIF:GPSAltitudeRef#=1", "-XMP-exif:GPSAltitude=30", "-XMP-exif:GPSAltitudeRef#=1"}},
		{0, 0, 0, []string{"-EXIF:GPSLatitudeRef=N", "-EXIF:GPSLongitudeRef=E", "-EXIF:GPSAltitudeRef#=0"}},
	}
	for _, c := range cases {
		args := buildWriteArgs(domain.Metadata{Latitude: &c.lat, Longitude: &c.lon, Altitude: &c.alt}, WriteOptions{})
		for _, w := range c.want {
			if !has(args, w) {
				t.Errorf("lat=%v lon=%v alt=%v: missing %s in %v", c.lat, c.lon, c.alt, w, args)
			}
		}
	}
	noAlt := buildWriteArgs(domain.Metadata{Latitude: f64(1), Longitude: f64(2)}, WriteOptions{})
	for _, a := range noAlt {
		if strings.Contains(a, "Altitude") {
			t.Errorf("no altitude requested, got %s", a)
		}
	}
}

func TestRatingWritesPercentToo(t *testing.T) {
	for r, pct := range map[int]string{0: "0", 1: "1", 2: "25", 3: "50", 4: "75", 5: "99"} {
		args := buildWriteArgs(domain.Metadata{Rating: ip(r)}, WriteOptions{})
		if !has(args, "-IFD0:RatingPercent="+pct) || !has(args, "-XMP-xmp:Rating="+string(rune('0'+r))) {
			t.Errorf("rating %d: %v", r, args)
		}
	}
}

func TestDateWithAndWithoutZone(t *testing.T) {
	z := buildWriteArgs(domain.Metadata{DateTaken: "2026-05-01T10:00:00+06:00"}, WriteOptions{})
	for _, w := range []string{
		"-ExifIFD:DateTimeOriginal=2026:05:01 10:00:00", "-ExifIFD:CreateDate=2026:05:01 10:00:00",
		"-XMP-exif:DateTimeOriginal=2026:05:01 10:00:00+06:00", "-XMP-photoshop:DateCreated=2026:05:01 10:00:00+06:00",
		"-IPTC:DateCreated=2026:05:01", "-IPTC:TimeCreated=10:00:00+06:00",
		"-ExifIFD:OffsetTimeOriginal=+06:00", "-ExifIFD:OffsetTimeDigitized=+06:00"} {
		if !has(z, w) {
			t.Errorf("missing %s", w)
		}
	}
	for _, a := range buildWriteArgs(domain.Metadata{DateTaken: "2026-05-01"}, WriteOptions{}) {
		if strings.Contains(a, "OffsetTime") {
			t.Errorf("no zone given, got %s", a)
		}
	}
	if !has(buildWriteArgs(domain.Metadata{DateTaken: "2026-05-01"}, WriteOptions{}), "-ExifIFD:DateTimeOriginal=2026:05:01 00:00:00") {
		t.Error("date-only must be normalised")
	}
}

func TestKeywordOperators(t *testing.T) {
	rep := buildWriteArgs(domain.Metadata{Keywords: []string{"a", "b"}}, WriteOptions{})
	app := buildWriteArgs(domain.Metadata{Keywords: []string{"a"}}, WriteOptions{AppendKeywords: true})
	if !has(rep, "-IPTC:Keywords=a") || !has(rep, "-XMP-dc:Subject=b") {
		t.Errorf("replace: %v", rep)
	}
	if !has(app, "-IPTC:Keywords+=a") || !has(app, "-XMP-dc:Subject+=a") {
		t.Errorf("append: %v", app)
	}
	for _, a := range append(rep, app...) {
		if strings.Contains(a, "Keywords=") && strings.HasSuffix(strings.SplitN(a, "=", 2)[0], "+") != strings.Contains(a, "+=") {
			t.Errorf("odd keyword arg %s", a)
		}
	}
}

func TestStripAllRestoresICCAndOrientation(t *testing.T) {
	args := buildWriteArgs(domain.Metadata{Strip: "all", Creator: "X"}, WriteOptions{})
	want := []string{"-all=", "-tagsfromfile", "@", "-ICC_Profile", "-Orientation#"}
	if !reflect.DeepEqual(args[:5], want) {
		t.Errorf("strip all must come first and restore profile+orientation: %v", args)
	}
	if index(args, "-IFD0:Artist=X") < 5 {
		t.Error("new values must be written after the strip")
	}
}

func TestStripGPS(t *testing.T) {
	args := buildWriteArgs(domain.Metadata{Strip: "GPS"}, WriteOptions{})
	if !reflect.DeepEqual(args, []string{"-GPS:all=", "-XMP-exif:GPS*="}) {
		t.Errorf("%v", args)
	}
}

func TestOrderingStripRemoveNamedRaw(t *testing.T) {
	args := buildWriteArgs(domain.Metadata{
		Strip: "gps", Remove: []string{"Comment", "comment", "XMP:all"}, Creator: "N",
		Tags: map[string]domain.TagValue{"Artist": {"Raw"}, "Zed": {"z"}, "XMP-dc:Subject": {"a", "b"}, "Empty": {""}, "Nil": nil},
	}, WriteOptions{})
	iStrip, iRemove, iNamed, iRaw := index(args, "-GPS:all="), index(args, "-Comment="), index(args, "-IFD0:Artist=N"), index(args, "-Artist=Raw")
	if !(iStrip < iRemove && iRemove < iNamed && iNamed < iRaw) {
		t.Errorf("order must be strip < remove < named < raw: %d %d %d %d\n%v", iStrip, iRemove, iNamed, iRaw, args)
	}
	count := 0
	for _, a := range args {
		if strings.EqualFold(a, "-Comment=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("duplicate removals must collapse, got %d", count)
	}
	for _, w := range []string{"-XMP:all=", "-XMP-dc:Subject=a", "-XMP-dc:Subject=b", "-Empty=", "-Nil=", "-Zed=z"} {
		if !has(args, w) {
			t.Errorf("missing %s in %v", w, args)
		}
	}
}

func TestValuesAreSingleLineAndLiteral(t *testing.T) {
	args := buildWriteArgs(domain.Metadata{Title: "line1\nline2\r\nline3", Tags: map[string]domain.TagValue{"Comment": {"<file $x & `y`\n-all="}}}, WriteOptions{})
	for _, a := range args {
		if strings.ContainsAny(a, "\n\r") {
			t.Errorf("argument contains a newline (would inject a second argument): %q", a)
		}
	}
	if !has(args, "-IPTC:ObjectName=line1 line2 line3") {
		t.Errorf("%v", args)
	}
	if !has(args, "-Comment=<file $x & `y` -all=") {
		t.Errorf("raw values must stay on one line and be passed literally: %v", args)
	}
}

func TestParseCoordinate(t *testing.T) {
	cases := map[string]float64{
		`23 deg 48' 37.08" N`: 23.8103, `33 deg 54' 0.00" S`: -33.9, `18 deg 24' 0.00" W`: -18.4, `90 deg 24' 45.00" E`: 90.4125,
		"33.9 S": -33.9, "-33.9": -33.9, "12.5": 12.5, "0": 0, "10 N": 10,
	}
	for in, want := range cases {
		got, ok := parseCoordinate(in)
		if !ok || got < want-0.0001 || got > want+0.0001 {
			t.Errorf("%q => %v %v, want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "north", "x deg"} {
		if _, ok := parseCoordinate(bad); ok {
			t.Errorf("%q must not parse", bad)
		}
	}
}

func TestParseAltitude(t *testing.T) {
	cases := map[string]float64{"12.5 m": 12.5, "30 m Below Sea Level": -30, "30 m Above Sea Level": 30, "7": 7, "-3": -3}
	for in, want := range cases {
		if got, ok := parseAltitude(in); !ok || got != want {
			t.Errorf("%q => %v %v, want %v", in, got, ok, want)
		}
	}
	if _, ok := parseAltitude("high"); ok {
		t.Error("must not parse words")
	}
}

func TestSelectTags(t *testing.T) {
	m := map[string]any{
		"SourceFile": "x.jpg", "IFD0:Artist": "A", "XMP-dc:Creator": "A", "IPTC:Keywords": []any{"k"},
		"XMP-dc:Subject": []any{"k"}, "ExifIFD:DateTimeOriginal": "d", "File:FileSize": "1 kB",
	}
	keys := func(patterns ...string) []string {
		var out []string
		for k := range SelectTags(m, patterns) {
			out = append(out, k)
		}
		return out
	}
	if len(keys()) != 6 {
		t.Errorf("no patterns returns everything except SourceFile: %v", keys())
	}
	if got := keys("artist"); len(got) != 1 || got[0] != "IFD0:Artist" {
		t.Errorf("bare name matches the tag in any group, case-insensitively: %v", got)
	}
	if got := keys("xmp-dc:*"); len(got) != 2 {
		t.Errorf("group wildcard: %v", got)
	}
	if got := keys("*date*", "Keywords"); len(got) != 2 {
		t.Errorf("multiple patterns: %v", got)
	}
	if got := keys("IPTC:Artist"); len(got) != 0 {
		t.Errorf("group must match when given: %v", got)
	}
	if got := keys("Artist#"); len(got) != 1 {
		t.Errorf("trailing # ignored: %v", got)
	}
}
