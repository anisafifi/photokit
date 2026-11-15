package domain

import (
	"errors"
	"math"
	"sort"
	"strings"
)

// Contact is the photographer's contact information (IPTC "Creator contact info").
type Contact struct {
	Email      string `json:"email,omitempty" yaml:"email,omitempty"`
	Phone      string `json:"phone,omitempty" yaml:"phone,omitempty"`
	URL        string `json:"url,omitempty" yaml:"url,omitempty"`
	Address    string `json:"address,omitempty" yaml:"address,omitempty"`
	City       string `json:"city,omitempty" yaml:"city,omitempty"`
	Region     string `json:"region,omitempty" yaml:"region,omitempty"`
	PostalCode string `json:"postal_code,omitempty" yaml:"postal_code,omitempty"`
	Country    string `json:"country,omitempty" yaml:"country,omitempty"`
}

// IsEmpty reports whether no contact field is set.
func (c Contact) IsEmpty() bool { return c == Contact{} }

func (c Contact) override(o Contact) Contact {
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&c.Email, o.Email)
	set(&c.Phone, o.Phone)
	set(&c.URL, o.URL)
	set(&c.Address, o.Address)
	set(&c.City, o.City)
	set(&c.Region, o.Region)
	set(&c.PostalCode, o.PostalCode)
	set(&c.Country, o.Country)
	return c
}

// Strip modes.
const (
	StripNone = ""
	StripGPS  = "gps"
	StripAll  = "all"
)

// Metadata is the tool-independent metadata model. It deliberately knows
// nothing about EXIF/IPTC/XMP tag names; adapters map it explicitly.
//
// Named fields are mapped consistently to every standard that can hold them.
// Tags, Remove and Strip give direct access to everything ExifTool supports.
type Metadata struct {
	// Descriptive and rights
	Creator      string   `json:"creator,omitempty" yaml:"creator,omitempty"`
	Copyright    string   `json:"copyright,omitempty" yaml:"copyright,omitempty"`
	Rights       string   `json:"rights,omitempty" yaml:"rights,omitempty"`
	Title        string   `json:"title,omitempty" yaml:"title,omitempty"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	Keywords     []string `json:"keywords,omitempty" yaml:"keywords,omitempty"`
	Credit       string   `json:"credit,omitempty" yaml:"credit,omitempty"`
	Source       string   `json:"source,omitempty" yaml:"source,omitempty"`
	Instructions string   `json:"instructions,omitempty" yaml:"instructions,omitempty"`
	Headline     string   `json:"headline,omitempty" yaml:"headline,omitempty"`
	Writer       string   `json:"writer,omitempty" yaml:"writer,omitempty"` // caption writer
	Owner        string   `json:"owner,omitempty" yaml:"owner,omitempty"`   // camera owner
	WebStatement string   `json:"web_statement,omitempty" yaml:"web_statement,omitempty"`
	Label        string   `json:"label,omitempty" yaml:"label,omitempty"`
	Rating       *int     `json:"rating,omitempty" yaml:"rating,omitempty"`

	// Where and when
	Location    string   `json:"location,omitempty" yaml:"location,omitempty"` // sub-location
	City        string   `json:"city,omitempty" yaml:"city,omitempty"`
	State       string   `json:"state,omitempty" yaml:"state,omitempty"`
	Country     string   `json:"country,omitempty" yaml:"country,omitempty"`
	CountryCode string   `json:"country_code,omitempty" yaml:"country_code,omitempty"`
	Latitude    *float64 `json:"latitude,omitempty" yaml:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty" yaml:"longitude,omitempty"`
	Altitude    *float64 `json:"altitude,omitempty" yaml:"altitude,omitempty"` // metres, negative = below sea level
	DateTaken   string   `json:"date_taken,omitempty" yaml:"date_taken,omitempty"`

	Contact Contact `json:"contact,omitempty" yaml:"contact,omitempty"`

	// Direct ExifTool access.
	Tags   map[string]TagValue `json:"tags,omitempty" yaml:"tags,omitempty"`     // any writable tag
	Remove []string            `json:"remove,omitempty" yaml:"remove,omitempty"` // tags to delete
	Strip  string              `json:"strip,omitempty" yaml:"strip,omitempty"`   // "", "gps" or "all"
}

// IsEmpty reports whether nothing would be written, removed or stripped.
func (m Metadata) IsEmpty() bool {
	return m.Creator == "" && m.Copyright == "" && m.Rights == "" && m.Title == "" &&
		m.Description == "" && len(m.Keywords) == 0 && m.Credit == "" && m.Source == "" &&
		m.Instructions == "" && m.Headline == "" && m.Writer == "" && m.Owner == "" &&
		m.WebStatement == "" && m.Label == "" && m.Rating == nil &&
		m.Location == "" && m.City == "" && m.State == "" && m.Country == "" && m.CountryCode == "" &&
		m.Latitude == nil && m.Longitude == nil && m.Altitude == nil && m.DateTaken == "" &&
		m.Contact.IsEmpty() && len(m.Tags) == 0 && len(m.Remove) == 0 &&
		(m.Strip == StripNone || m.Strip == "none")
}

// Override returns m with every set field of o applied on top. Maps and
// slices are copied, so neither input is modified. Tags merge per tag, Remove
// accumulates, everything else is replaced when o sets it.
func (m Metadata) Override(o Metadata) Metadata {
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&m.Creator, o.Creator)
	set(&m.Copyright, o.Copyright)
	set(&m.Rights, o.Rights)
	set(&m.Title, o.Title)
	set(&m.Description, o.Description)
	set(&m.Credit, o.Credit)
	set(&m.Source, o.Source)
	set(&m.Instructions, o.Instructions)
	set(&m.Headline, o.Headline)
	set(&m.Writer, o.Writer)
	set(&m.Owner, o.Owner)
	set(&m.WebStatement, o.WebStatement)
	set(&m.Label, o.Label)
	set(&m.Location, o.Location)
	set(&m.City, o.City)
	set(&m.State, o.State)
	set(&m.Country, o.Country)
	set(&m.CountryCode, o.CountryCode)
	set(&m.DateTaken, o.DateTaken)
	set(&m.Strip, o.Strip)

	if len(o.Keywords) > 0 {
		m.Keywords = append([]string(nil), o.Keywords...)
	} else {
		m.Keywords = append([]string(nil), m.Keywords...)
	}
	if o.Rating != nil {
		v := *o.Rating
		m.Rating = &v
	} else if m.Rating != nil {
		v := *m.Rating
		m.Rating = &v
	}
	m.Latitude = pick(m.Latitude, o.Latitude)
	m.Longitude = pick(m.Longitude, o.Longitude)
	m.Altitude = pick(m.Altitude, o.Altitude)
	m.Contact = m.Contact.override(o.Contact)

	if len(m.Tags) > 0 || len(o.Tags) > 0 {
		tags := make(map[string]TagValue, len(m.Tags)+len(o.Tags))
		for k, v := range m.Tags {
			tags[k] = append(TagValue(nil), v...)
		}
		for k, v := range o.Tags {
			tags[k] = append(TagValue(nil), v...)
		}
		m.Tags = tags
	}
	if len(m.Remove) > 0 || len(o.Remove) > 0 {
		seen := map[string]bool{}
		var merged []string
		for _, r := range append(append([]string(nil), m.Remove...), o.Remove...) {
			if k := strings.ToLower(r); !seen[k] {
				seen[k] = true
				merged = append(merged, r)
			}
		}
		m.Remove = merged
	}
	return m
}

func pick(base, over *float64) *float64 {
	src := base
	if over != nil {
		src = over
	}
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

// SortedTagNames returns the keys of Tags in a stable order.
func (m Metadata) SortedTagNames() []string {
	names := make([]string, 0, len(m.Tags))
	for k := range m.Tags {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Validate checks every value that can be checked without ExifTool and
// reports all problems at once. (Whether a tag name really exists is checked
// against ExifTool itself, before any file is touched.)
func (m Metadata) Validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, Invalidf(format, args...)) }

	for i, k := range m.Keywords {
		if strings.TrimSpace(k) == "" {
			bad("metadata.keywords[%d] is empty", i)
		}
	}
	if m.Rating != nil && (*m.Rating < 0 || *m.Rating > 5) {
		bad("metadata.rating must be between 0 and 5, got %d", *m.Rating)
	}
	if v := m.Latitude; v != nil && (math.IsNaN(*v) || *v < -90 || *v > 90) {
		bad("metadata.latitude must be between -90 and 90, got %v", *v)
	}
	if v := m.Longitude; v != nil && (math.IsNaN(*v) || *v < -180 || *v > 180) {
		bad("metadata.longitude must be between -180 and 180, got %v", *v)
	}
	if v := m.Altitude; v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
		bad("metadata.altitude must be a finite number, got %v", *v)
	}
	if (m.Latitude == nil) != (m.Longitude == nil) {
		bad("metadata.latitude and metadata.longitude must be given together")
	}
	if m.Altitude != nil && m.Latitude == nil {
		bad("metadata.altitude needs metadata.latitude and metadata.longitude")
	}
	if m.DateTaken != "" {
		if _, err := ParseDateTime(m.DateTaken); err != nil {
			bad("metadata.date_taken: %v", err)
		}
	}
	if len(m.CountryCode) > 0 && len(strings.TrimSpace(m.CountryCode)) != 2 && len(strings.TrimSpace(m.CountryCode)) != 3 {
		bad("metadata.country_code must be an ISO 3166 code such as BD or BGD, got %q", m.CountryCode)
	}
	switch strings.ToLower(m.Strip) {
	case "", "none", StripGPS, StripAll:
	default:
		bad("metadata.strip must be one of: none, gps, all (got %q)", m.Strip)
	}
	for _, name := range m.SortedTagNames() {
		if err := ValidateSetTag(name); err != nil {
			bad("metadata.tags.%s: %v", name, err)
		}
	}
	for i, name := range m.Remove {
		if err := ValidateRemoveTag(name); err != nil {
			bad("metadata.remove[%d]: %v", i, err)
		}
	}
	return errors.Join(errs...)
}

// Info is a read-only description of a photo, used by the inspect commands.
type Info struct {
	Path      string         `json:"path"`
	Format    string         `json:"format,omitempty"`
	Width     int            `json:"width,omitempty"`
	Height    int            `json:"height,omitempty"`
	Camera    string         `json:"camera,omitempty"`
	Lens      string         `json:"lens,omitempty"`
	DateTaken string         `json:"date_taken,omitempty"`
	Metadata  Metadata       `json:"metadata"`
	Tags      map[string]any `json:"tags,omitempty"` // every tag ExifTool found (--all / --tag)
	Error     string         `json:"error,omitempty"`
}
