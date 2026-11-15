package metadata_test

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"photokit/internal/domain"
	"photokit/internal/metadata"
	"photokit/internal/testutil"
)

func f64(v float64) *float64 { return &v }
func ip(v int) *int          { return &v }

func near(a, b *float64) bool { return a != nil && b != nil && math.Abs(*a-*b) < 0.0005 }

func exif(t *testing.T, et *metadata.ExifTool, args ...string) string {
	t.Helper()
	out, err := exec.Command(et.Path, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("exiftool %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newJPEG(t *testing.T) string {
	return testutil.WriteJPEG(t, filepath.Join(t.TempDir(), "a.jpg"), 64, 48)
}

func TestAllNamedFieldsRoundTrip(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	for name, mk := range map[string]func(testing.TB, string, int, int) string{"jpeg": testutil.WriteJPEG, "png": testutil.WritePNG} {
		t.Run(name, func(t *testing.T) {
			path := mk(t, filepath.Join(t.TempDir(), "a."+map[string]string{"jpeg": "jpg", "png": "png"}[name]), 64, 48)
			want := domain.Metadata{
				Creator: "Zoë", Copyright: "© 2026 Zoë", Rights: "All rights reserved", Title: "T", Description: "D",
				Keywords: []string{"a", "b, c"}, Credit: "Cr", Source: "S", Instructions: "I",
				Headline: "Headline", Writer: "Writer", Owner: "Owner", WebStatement: "https://example.com/license",
				Label: "Red", Rating: ip(4),
				Location: "Ramna", City: "Dhaka", State: "Dhaka Division", Country: "Bangladesh", CountryCode: "BD",
				Latitude: f64(23.8103), Longitude: f64(90.4125), Altitude: f64(12.5),
				DateTaken: "2026-05-01T10:30:15+06:00",
				Contact: domain.Contact{Email: "a@b.c", Phone: "+8801700000000", URL: "https://x.y", Address: "1 Main St",
					City: "Dhaka", Region: "Dhaka", PostalCode: "1207", Country: "Bangladesh"},
			}
			if err := et.Write(ctx, path, want, metadata.WriteOptions{}); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := et.Read(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range []struct{ field, got, want string }{
				{"creator", got.Creator, want.Creator}, {"copyright", got.Copyright, want.Copyright}, {"rights", got.Rights, want.Rights},
				{"title", got.Title, want.Title}, {"description", got.Description, want.Description}, {"credit", got.Credit, want.Credit},
				{"source", got.Source, want.Source}, {"instructions", got.Instructions, want.Instructions},
				{"headline", got.Headline, want.Headline}, {"writer", got.Writer, want.Writer}, {"web_statement", got.WebStatement, want.WebStatement},
				{"label", got.Label, want.Label}, {"location", got.Location, want.Location}, {"city", got.City, want.City},
				{"state", got.State, want.State}, {"country", got.Country, want.Country}, {"country_code", got.CountryCode, want.CountryCode},
				{"contact.email", got.Contact.Email, want.Contact.Email}, {"contact.phone", got.Contact.Phone, want.Contact.Phone},
				{"contact.url", got.Contact.URL, want.Contact.URL}, {"contact.address", got.Contact.Address, want.Contact.Address},
				{"contact.city", got.Contact.City, want.Contact.City}, {"contact.region", got.Contact.Region, want.Contact.Region},
				{"contact.postal_code", got.Contact.PostalCode, want.Contact.PostalCode}, {"contact.country", got.Contact.Country, want.Contact.Country},
			} {
				if c.got != c.want {
					t.Errorf("%s: got %q, want %q", c.field, c.got, c.want)
				}
			}
			if !reflect.DeepEqual(got.Keywords, want.Keywords) {
				t.Errorf("keywords: %v", got.Keywords)
			}
			if got.Rating == nil || *got.Rating != 4 {
				t.Errorf("rating: %v", got.Rating)
			}
			if !near(got.Latitude, want.Latitude) || !near(got.Longitude, want.Longitude) || !near(got.Altitude, want.Altitude) {
				t.Errorf("gps: %v %v %v", deref(got.Latitude), deref(got.Longitude), deref(got.Altitude))
			}
			if !strings.HasPrefix(got.DateTaken, "2026:05:01 10:30:15") {
				t.Errorf("date: %q", got.DateTaken)
			}
			if name == "jpeg" {
				if o := exif(t, et, "-s3", "-ExifIFD:OwnerName", path); o != "Owner" {
					t.Errorf("owner: %q", o)
				}
				if got.Owner != "Owner" {
					t.Errorf("owner read: %q", got.Owner)
				}
				if o := exif(t, et, "-s3", "-ExifIFD:OffsetTimeOriginal", path); o != "+06:00" {
					t.Errorf("time zone offset: %q", o)
				}
			}
		})
	}
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestGPSHemispheresAndAltitudeSign(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	cases := []struct {
		name          string
		lat, lon, alt float64
		refs          string
	}{
		{"north-east above", 23.8103, 90.4125, 12.5, "N E 0"},
		{"south-west below", -33.9249, -18.4241, -30, "S W 1"},
		{"north-west", 40.7128, -74.006, 10, "N W 0"},
		{"south-east", -37.8136, 144.9631, 31, "S E 0"},
		{"null island", 0, 0, 0, "N E 0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := newJPEG(t)
			md := domain.Metadata{Latitude: f64(c.lat), Longitude: f64(c.lon), Altitude: f64(c.alt)}
			if err := et.Write(ctx, path, md, metadata.WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			got, err := et.Read(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if !near(got.Latitude, &c.lat) || !near(got.Longitude, &c.lon) || !near(got.Altitude, &c.alt) {
				t.Errorf("read back %v %v %v, wrote %v %v %v", deref(got.Latitude), deref(got.Longitude), deref(got.Altitude), c.lat, c.lon, c.alt)
			}
			refs := exif(t, et, "-s3", "-GPS:GPSLatitudeRef", "-GPS:GPSLongitudeRef", "-n", "-GPS:GPSAltitudeRef", path)
			if strings.Join(strings.Fields(refs), " ") != c.refs {
				t.Errorf("EXIF refs %q, want %q", refs, c.refs)
			}
			// XMP carries the signed decimal
			x := exif(t, et, "-s3", "-n", "-XMP-exif:GPSLatitude", path)
			if xv, err := strconv.ParseFloat(x, 64); err != nil || math.Abs(xv-c.lat) > 0.0005 {
				t.Errorf("XMP latitude %q for %v", x, c.lat)
			}
		})
	}
}

func TestRatingRange(t *testing.T) {
	et := testutil.RequireExifTool(t)
	for _, r := range []int{0, 1, 3, 5} {
		path := newJPEG(t)
		if err := et.Write(context.Background(), path, domain.Metadata{Rating: ip(r)}, metadata.WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, _ := et.Read(context.Background(), path)
		if got.Rating == nil || *got.Rating != r {
			t.Errorf("rating %d read back as %v", r, got.Rating)
		}
	}
}

func TestDateOnlyAndZone(t *testing.T) {
	et := testutil.RequireExifTool(t)
	path := newJPEG(t)
	if err := et.Write(context.Background(), path, domain.Metadata{DateTaken: "2026-05-01"}, metadata.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := et.Read(context.Background(), path); got.DateTaken != "2026:05:01 00:00:00" {
		t.Errorf("date-only: %q", got.DateTaken)
	}
	if v := exif(t, et, "-s3", "-IPTC:DateCreated", "-IPTC:TimeCreated", path); !strings.Contains(v, "2026:05:01") {
		t.Errorf("IPTC date: %q", v)
	}
}

func TestStripAllKeepsOrientationAndWritesNewValues(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	path := newJPEG(t)
	testutil.Exif(t, et, path, "-Make=ACME", "-Model=X1", "-Artist=Old", "-Orientation#=6", "-GPSLatitude=1", "-GPSLatitudeRef=N", "-GPSLongitude=2", "-GPSLongitudeRef=E", "-XMP-dc:Creator=Old", "-Comment=old comment")

	err := et.Write(ctx, path, domain.Metadata{Strip: "all", Creator: "Fresh", Copyright: "© New"}, metadata.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info, _ := et.Inspect(ctx, path, metadata.InspectOptions{})
	if info.Camera != "" || info.Metadata.Latitude != nil {
		t.Errorf("strip must remove camera and GPS: %+v", info)
	}
	if info.Metadata.Creator != "Fresh" || info.Metadata.Copyright != "© New" {
		t.Errorf("values written after the strip must survive: %+v", info.Metadata)
	}
	if c := exif(t, et, "-s3", "-Comment", path); c != "" {
		t.Errorf("comment must be gone: %q", c)
	}
	if o := exif(t, et, "-s3", "-n", "-Orientation", path); o != "6" {
		t.Errorf("orientation must be preserved so the picture is not rotated: %q", o)
	}
}

func TestStripGPSOnly(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	path := newJPEG(t)
	if err := et.Write(ctx, path, domain.Metadata{Creator: "Keep", Latitude: f64(10), Longitude: f64(20), Altitude: f64(5)}, metadata.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := et.Write(ctx, path, domain.Metadata{Strip: "gps"}, metadata.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, _ := et.Read(ctx, path)
	if got.Latitude != nil || got.Longitude != nil || got.Altitude != nil {
		t.Errorf("GPS must be gone from EXIF and XMP: %v %v %v", deref(got.Latitude), deref(got.Longitude), deref(got.Altitude))
	}
	if left := exif(t, et, "-G1", "-a", "-s", "-GPS:all", "-XMP-exif:GPS*", path); left != "" {
		t.Errorf("leftover GPS tags: %s", left)
	}
	if got.Creator != "Keep" {
		t.Error("strip gps must not touch other metadata")
	}
}

func TestRemoveTags(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	path := newJPEG(t)
	testutil.Exif(t, et, path, "-Comment=bye", "-Artist=A", "-XMP-dc:Creator=A", "-XMP-dc:Title=T", "-IPTC:Keywords=k")
	err := et.Write(ctx, path, domain.Metadata{Remove: []string{"Comment", "IFD0:Artist", "XMP-dc:*"}}, metadata.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if left := exif(t, et, "-s3", "-Comment", "-IFD0:Artist", "-XMP-dc:all", path); left != "" {
		t.Errorf("removed tags still present: %q", left)
	}
	if k := exif(t, et, "-s3", "-IPTC:Keywords", path); k != "k" {
		t.Errorf("unrelated tags must survive: %q", k)
	}
}

func TestRawTagsAnyWritableTag(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	path := newJPEG(t)
	md := domain.Metadata{
		Creator: "Named",
		Tags: map[string]domain.TagValue{
			"Comment":             {"hello wörld"},
			"XMP-dc:Subject":      {"x", "y"},
			"Orientation#":        {"3"},
			"ExifIFD:UserComment": {"uc"},
			"IFD0:Artist":         {"Raw wins"},
			"XMP-xmp:Rating":      {"2"},
		},
	}
	if err := et.CheckTags(ctx, md); err != nil {
		t.Fatalf("CheckTags: %v", err)
	}
	if err := et.Write(ctx, path, md, metadata.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	checks := map[string][]string{
		"-s3 -Comment":             {"hello wörld"},
		"-s3 -n -Orientation":      {"3"},
		"-s3 -ExifIFD:UserComment": {"uc"},
		"-s3 -IFD0:Artist":         {"Raw wins"},
		"-s3 -n -XMP-xmp:Rating":   {"2"},
		"-s3 -XMP-dc:Subject":      {"x, y"},
		"-s3 -IPTC:By-line":        {"Named"},
	}
	for args, want := range checks {
		if got := exif(t, et, append(strings.Fields(args), path)...); got != want[0] {
			t.Errorf("%s = %q, want %q", args, got, want[0])
		}
	}
}

func TestRawTagEmptyValueDeletes(t *testing.T) {
	et := testutil.RequireExifTool(t)
	path := newJPEG(t)
	testutil.Exif(t, et, path, "-Comment=x")
	if err := et.Write(context.Background(), path, domain.Metadata{Tags: map[string]domain.TagValue{"Comment": nil}}, metadata.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if c := exif(t, et, "-s3", "-Comment", path); c != "" {
		t.Errorf("empty value must delete the tag, got %q", c)
	}
}

func TestRawValuesAreNeverInterpreted(t *testing.T) {
	et := testutil.RequireExifTool(t)
	dir := t.TempDir()
	path := testutil.WriteJPEG(t, filepath.Join(dir, "a.jpg"), 32, 32)
	nasty := "<" + filepath.Join(dir, "a.jpg") + " $Make `id` ; -all= | rm -rf ~"
	if err := et.Write(context.Background(), path, domain.Metadata{Tags: map[string]domain.TagValue{"Comment": {nasty}}}, metadata.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if c := exif(t, et, "-s3", "-Comment", path); c != nasty {
		t.Errorf("value must be stored literally:\n got %q\nwant %q", c, nasty)
	}
	if _, err := testutilStat(path); err != nil {
		t.Error("file vanished")
	}
}

func TestCheckTagsRejectsUnknownBeforeAnyWrite(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	err := et.CheckTags(ctx, domain.Metadata{
		Tags:   map[string]domain.TagValue{"Artist": {"ok"}, "Artsit": {"typo"}, "csv": {"x"}, "Totally:Bogus": {"y"}},
		Remove: []string{"Comment", "XMP:all", "GPS*", "NopeNope"},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{`"Artsit"`, `"csv"`, `"Totally:Bogus"`, `"NopeNope"`, "photokit metadata tags"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %s in:\n%v", want, err)
		}
	}
	for _, notWant := range []string{`"Artist"`, `"Comment"`, `"XMP:all"`, `"GPS*"`} {
		if strings.Contains(err.Error(), notWant) {
			t.Errorf("valid tag %s was reported:\n%v", notWant, err)
		}
	}
	if !strings.Contains(err.Error(), "did you mean") || !strings.Contains(strings.ToLower(err.Error()), "artist") {
		t.Errorf("a near miss should come with a suggestion:\n%v", err)
	}
	if err := et.CheckTags(ctx, domain.Metadata{Creator: "x"}); err != nil {
		t.Errorf("named fields need no tag check: %v", err)
	}
}

func TestInspectAllAndFiltered(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	path := newJPEG(t)
	testutil.Exif(t, et, path, "-Artist=Alice", "-XMP-dc:Creator=Alice", "-Make=ACME")

	plain, err := et.Inspect(ctx, path, metadata.InspectOptions{})
	if err != nil || plain.Tags != nil {
		t.Fatalf("tags must only be returned on request: %v %v", plain.Tags, err)
	}
	all, err := et.Inspect(ctx, path, metadata.InspectOptions{All: true})
	if err != nil || len(all.Tags) < 10 {
		t.Fatalf("--all: %d tags, %v", len(all.Tags), err)
	}
	for _, k := range []string{"IFD0:Artist", "XMP-dc:Creator", "IFD0:Make", "File:FileType"} {
		if _, ok := all.Tags[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	if _, ok := all.Tags["SourceFile"]; ok {
		t.Error("SourceFile is noise")
	}
	f, err := et.Inspect(ctx, path, metadata.InspectOptions{Tags: []string{"artist", "xmp-dc:*"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Tags) != 2 || f.Tags["IFD0:Artist"] != "Alice" || f.Tags["XMP-dc:Creator"] != "Alice" {
		t.Errorf("filtered tags: %v", f.Tags)
	}
}

func TestListTags(t *testing.T) {
	et := testutil.RequireExifTool(t)
	ctx := context.Background()
	dc, err := et.ListTags(ctx, "XMP-dc")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, ti := range dc {
		if !strings.EqualFold(ti.Group, "XMP-dc") {
			t.Errorf("group filter leaked %s:%s", ti.Group, ti.Name)
		}
		names[ti.Name] = true
	}
	for _, want := range []string{"Creator", "Subject", "Rights", "Title"} {
		if !names[want] {
			t.Errorf("XMP-dc is missing %s", want)
		}
	}
	iptc, err := et.ListTags(ctx, "iptc")
	if err != nil || len(iptc) < 20 {
		t.Fatalf("IPTC: %d tags, %v", len(iptc), err)
	}
	found := false
	for _, ti := range iptc {
		if ti.Name == "Keywords" {
			found = true
		}
	}
	if !found {
		t.Error("IPTC Keywords missing")
	}
	for i := 1; i < len(dc); i++ {
		if strings.ToLower(dc[i-1].Name) > strings.ToLower(dc[i].Name) {
			t.Fatal("not sorted")
		}
	}
}

func testutilStat(path string) (os.FileInfo, error) { return os.Stat(path) }
