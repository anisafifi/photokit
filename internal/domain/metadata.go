package domain

// Metadata is the tool-independent metadata model. It deliberately knows
// nothing about EXIF/IPTC/XMP tag names; adapters map it explicitly.
type Metadata struct {
	Creator      string   `json:"creator,omitempty" yaml:"creator,omitempty"`
	Copyright    string   `json:"copyright,omitempty" yaml:"copyright,omitempty"`
	Rights       string   `json:"rights,omitempty" yaml:"rights,omitempty"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	Title        string   `json:"title,omitempty" yaml:"title,omitempty"`
	Keywords     []string `json:"keywords,omitempty" yaml:"keywords,omitempty"`
	Credit       string   `json:"credit,omitempty" yaml:"credit,omitempty"`
	Source       string   `json:"source,omitempty" yaml:"source,omitempty"`
	Instructions string   `json:"instructions,omitempty" yaml:"instructions,omitempty"`
}

// IsEmpty reports whether no field is set.
func (m Metadata) IsEmpty() bool {
	return m.Creator == "" && m.Copyright == "" && m.Rights == "" &&
		m.Description == "" && m.Title == "" && len(m.Keywords) == 0 &&
		m.Credit == "" && m.Source == "" && m.Instructions == ""
}

// Override returns m with every non-empty field of o applied on top.
func (m Metadata) Override(o Metadata) Metadata {
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&m.Creator, o.Creator)
	set(&m.Copyright, o.Copyright)
	set(&m.Rights, o.Rights)
	set(&m.Description, o.Description)
	set(&m.Title, o.Title)
	set(&m.Credit, o.Credit)
	set(&m.Source, o.Source)
	set(&m.Instructions, o.Instructions)
	if len(o.Keywords) > 0 {
		m.Keywords = append([]string(nil), o.Keywords...)
	}
	return m
}

// Info is a read-only description of a photo, used by the inspect commands.
type Info struct {
	Path      string   `json:"path"`
	Format    string   `json:"format,omitempty"`
	Width     int      `json:"width,omitempty"`
	Height    int      `json:"height,omitempty"`
	Camera    string   `json:"camera,omitempty"`
	Lens      string   `json:"lens,omitempty"`
	DateTaken string   `json:"date_taken,omitempty"`
	Metadata  Metadata `json:"metadata"`
	Error     string   `json:"error,omitempty"`
}
