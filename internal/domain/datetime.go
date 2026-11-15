package domain

import (
	"fmt"
	"strings"
	"time"
)

// DateTime is a parsed capture date, normalised for ExifTool.
type DateTime struct {
	Date string // 2026:05:01
	Time string // 10:00:00
	Zone string // +06:00, or "" when the input had no time zone
}

var dateLayouts = []struct {
	layout  string
	hasZone bool
	dateTim bool
}{
	{"2006:01:02 15:04:05Z07:00", true, true},
	{"2006-01-02 15:04:05Z07:00", true, true},
	{"2006-01-02T15:04:05Z07:00", true, true},
	{"2006:01:02 15:04:05", false, true},
	{"2006-01-02 15:04:05", false, true},
	{"2006-01-02T15:04:05", false, true},
	{"2006-01-02 15:04", false, true},
	{"2006-01-02T15:04", false, true},
	{"2006-01-02", false, false},
	{"2006:01:02", false, false},
}

// ParseDateTime accepts the common spellings of a capture date:
//
//	2026:05:01 10:00:00     (EXIF)
//	2026-05-01 10:00:00
//	2026-05-01T10:00:00+06:00
//	2026-05-01              (time becomes 00:00:00)
func ParseDateTime(s string) (DateTime, error) {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		t, err := time.Parse(l.layout, s)
		if err != nil {
			continue
		}
		d := DateTime{Date: t.Format("2006:01:02"), Time: t.Format("15:04:05")}
		if l.hasZone {
			d.Zone = t.Format("-07:00")
		}
		return d, nil
	}
	return DateTime{}, fmt.Errorf("cannot understand date %q (use e.g. 2026-05-01 10:00:00, 2026-05-01T10:00:00+06:00 or 2026-05-01)", s)
}

// Local returns "2006:01:02 15:04:05", the EXIF form without a zone.
func (d DateTime) Local() string { return d.Date + " " + d.Time }

// WithZone returns Local plus the zone when there is one.
func (d DateTime) WithZone() string { return d.Local() + d.Zone }
