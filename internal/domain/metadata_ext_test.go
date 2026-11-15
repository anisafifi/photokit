package domain

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func f64(v float64) *float64 { return &v }
func ip(v int) *int          { return &v }

func TestParseDateTime(t *testing.T) {
	cases := []struct {
		in, local, zone string
		bad             bool
	}{
		{"2026:05:01 10:00:00", "2026:05:01 10:00:00", "", false},
		{"2026-05-01 10:00:00", "2026:05:01 10:00:00", "", false},
		{"2026-05-01T10:00:00", "2026:05:01 10:00:00", "", false},
		{"2026-05-01T10:00:00+06:00", "2026:05:01 10:00:00", "+06:00", false},
		{"2026-05-01T10:00:00Z", "2026:05:01 10:00:00", "+00:00", false},
		{"2026:05:01 10:00:00-05:30", "2026:05:01 10:00:00", "-05:30", false},
		{"2026-05-01T10:00:00.123+02:00", "2026:05:01 10:00:00", "+02:00", false},
		{"2026-05-01 10:00", "2026:05:01 10:00:00", "", false},
		{"2026-05-01", "2026:05:01 00:00:00", "", false},
		{"  2026:05:01  ", "2026:05:01 00:00:00", "", false},
		{"yesterday", "", "", true},
		{"2026-13-01", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		d, err := ParseDateTime(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("%q must be rejected", c.in)
			}
			continue
		}
		if err != nil || d.Local() != c.local || d.Zone != c.zone {
			t.Errorf("%q => %+v, %v; want %s %s", c.in, d, err, c.local, c.zone)
		}
	}
	d, _ := ParseDateTime("2026-05-01T10:00:00+06:00")
	if d.WithZone() != "2026:05:01 10:00:00+06:00" {
		t.Errorf("WithZone = %s", d.WithZone())
	}
}

func TestTagNameValidation(t *testing.T) {
	for _, ok := range []string{"Artist", "XMP-dc:Rights", "EXIF:Artist", "Orientation#", "Exif:GPS:GPSLatitude", "Sub-location", "Country-PrimaryLocationName", "AEB_Shot"} {
		if err := ValidateSetTag(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-all", "Artist=x", "a b", "csv=foo", "@", "../x", "Artist\nx", "XMP-dc:", ":Artist", "A:B:C:D", "1Artist", "all", "XMP:all", "Artist*", "x;y", "-execute"} {
		if err := ValidateSetTag(bad); err == nil {
			t.Errorf("%q must be rejected for writing", bad)
		}
	}
	for _, ok := range []string{"Comment", "XMP:all", "all", "GPS:all", "XMP-exif:GPS*", "*", "IPTC:Keyw?rds"} {
		if err := ValidateRemoveTag(ok); err != nil {
			t.Errorf("%q should be valid for removal: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a b", "x=y", "-all", "@", "a;b", "$(x)"} {
		if err := ValidateRemoveTag(bad); err == nil {
			t.Errorf("%q must be rejected for removal", bad)
		}
	}
	if g, tag, num := SplitTag("XMP-dc:Subject"); g != "XMP-dc" || tag != "Subject" || num {
		t.Error("SplitTag group")
	}
	if g, tag, num := SplitTag("Orientation#"); g != "" || tag != "Orientation" || !num {
		t.Error("SplitTag numeric")
	}
	if g, tag, _ := SplitTag("Exif:GPS:GPSLatitude"); g != "Exif:GPS" || tag != "GPSLatitude" {
		t.Error("SplitTag two-level")
	}
}

func TestTagValueYAML(t *testing.T) {
	var m Metadata
	doc := `
tags:
  Comment: hello world
  Rating#: 4
  Flag: true
  XMP-dc:Subject: [one, 2, "three"]
  Empty:
  Date: 2026-05-01
`
	if err := yaml.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatal(err)
	}
	want := map[string]TagValue{
		"Comment": {"hello world"}, "Rating#": {"4"}, "Flag": {"true"},
		"XMP-dc:Subject": {"one", "2", "three"}, "Empty": nil, "Date": {"2026-05-01"}, // a null value decodes to nil, which means "delete the tag"
	}
	if !reflect.DeepEqual(m.Tags, want) {
		t.Errorf("got %#v", m.Tags)
	}
	for _, bad := range []string{"tags:\n  X: {a: b}\n", "tags:\n  X: [[1]]\n"} {
		var x Metadata
		if err := yaml.Unmarshal([]byte(bad), &x); err == nil {
			t.Errorf("must reject %q", bad)
		}
	}
	out, err := yaml.Marshal(Metadata{Tags: map[string]TagValue{"A": {"x"}, "B": {"y", "z"}}})
	if err != nil || !strings.Contains(string(out), "A: x") || !strings.Contains(string(out), "- z") {
		t.Errorf("marshal: %s %v", out, err)
	}
}

func TestMetadataValidate(t *testing.T) {
	ok := Metadata{Rating: ip(5), Latitude: f64(-90), Longitude: f64(180), Altitude: f64(-400), DateTaken: "2026-05-01", CountryCode: "bd", Strip: "GPS",
		Tags: map[string]TagValue{"Comment": {"x"}}, Remove: []string{"XMP:all"}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid metadata rejected: %v", err)
	}
	cases := map[string]Metadata{
		"rating high":     {Rating: ip(6)},
		"rating negative": {Rating: ip(-1)},
		"lat range":       {Latitude: f64(91), Longitude: f64(0)},
		"lon range":       {Latitude: f64(0), Longitude: f64(-181)},
		"lat without lon": {Latitude: f64(1)},
		"lon without lat": {Longitude: f64(1)},
		"alt without gps": {Altitude: f64(5)},
		"bad date":        {DateTaken: "someday"},
		"country code":    {CountryCode: "Bangladesh"},
		"bad strip":       {Strip: "everything"},
		"bad tag":         {Tags: map[string]TagValue{"a b": {"x"}}},
		"all tag":         {Tags: map[string]TagValue{"all": {"x"}}},
		"bad remove":      {Remove: []string{"x=y"}},
		"empty keyword":   {Keywords: []string{"ok", " "}},
	}
	for name, md := range cases {
		if err := md.Validate(); err == nil || !strings.Contains(err.Error(), "metadata.") {
			t.Errorf("%s: expected a metadata error, got %v", name, err)
		}
	}
	all := Metadata{Rating: ip(9), DateTaken: "x", Strip: "x"}
	err := all.Validate()
	for _, want := range []string{"rating", "date_taken", "strip"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("all problems must be reported together; missing %q in %v", want, err)
		}
	}
}

func TestMetadataIsEmptyCoversEveryField(t *testing.T) {
	if !(Metadata{}).IsEmpty() || !(Metadata{Strip: "none"}).IsEmpty() {
		t.Error("zero value must be empty")
	}
	nonEmpty := []Metadata{
		{Creator: "x"}, {Headline: "x"}, {Writer: "x"}, {Owner: "x"}, {WebStatement: "x"}, {Label: "x"}, {Rating: ip(0)},
		{Location: "x"}, {City: "x"}, {State: "x"}, {Country: "x"}, {CountryCode: "x"},
		{Latitude: f64(0)}, {Longitude: f64(0)}, {Altitude: f64(0)}, {DateTaken: "x"},
		{Contact: Contact{Email: "x"}}, {Tags: map[string]TagValue{"a": {"b"}}}, {Remove: []string{"a"}}, {Strip: "gps"},
	}
	for i, m := range nonEmpty {
		if m.IsEmpty() {
			t.Errorf("case %d must not be empty: %+v", i, m)
		}
	}
}

func TestOverrideNewFields(t *testing.T) {
	base := Metadata{
		City: "Dhaka", Rating: ip(3), Latitude: f64(1), Longitude: f64(2),
		Contact: Contact{Email: "a@b.c", City: "Dhaka"},
		Tags:    map[string]TagValue{"Comment": {"old"}, "Keep": {"k"}},
		Remove:  []string{"XMP:all"}, Strip: "gps",
	}
	over := Metadata{
		City: "Khulna", Rating: ip(5), Latitude: f64(9),
		Contact: Contact{Phone: "123", City: "Khulna"},
		Tags:    map[string]TagValue{"Comment": {"new"}, "Extra": {"e"}},
		Remove:  []string{"xmp:ALL", "Comment"}, Strip: "all",
	}
	got := base.Override(over)
	if got.City != "Khulna" || *got.Rating != 5 || *got.Latitude != 9 || *got.Longitude != 2 {
		t.Errorf("scalars: %+v", got)
	}
	if got.Contact != (Contact{Email: "a@b.c", Phone: "123", City: "Khulna"}) {
		t.Errorf("contact merges per field: %+v", got.Contact)
	}
	if !reflect.DeepEqual(got.Tags, map[string]TagValue{"Comment": {"new"}, "Keep": {"k"}, "Extra": {"e"}}) {
		t.Errorf("tags merge per tag: %v", got.Tags)
	}
	if !reflect.DeepEqual(got.Remove, []string{"XMP:all", "Comment"}) {
		t.Errorf("remove accumulates without case-insensitive duplicates: %v", got.Remove)
	}
	if got.Strip != "all" {
		t.Errorf("strip: %s", got.Strip)
	}
	// inputs untouched, outputs independent
	if base.City != "Dhaka" || *base.Rating != 3 || base.Tags["Comment"][0] != "old" || len(base.Remove) != 1 {
		t.Error("Override mutated the receiver")
	}
	*got.Rating = 1
	got.Tags["Keep"][0] = "changed"
	if *base.Rating != 3 || base.Tags["Keep"][0] != "k" || *over.Rating != 5 {
		t.Error("result shares memory with its inputs")
	}
	if !reflect.DeepEqual(Metadata{}.Override(Metadata{}), Metadata{Keywords: []string{}}) && !(Metadata{}.Override(Metadata{})).IsEmpty() {
		t.Error("empty override must stay empty")
	}
}
