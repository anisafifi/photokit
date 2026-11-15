package domain

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// TagValue is the value of a raw ExifTool tag: one value, or a list for
// list-type tags. In YAML it may be written as a scalar or as a sequence.
type TagValue []string

// UnmarshalYAML accepts a scalar (any type, kept as written) or a sequence of
// scalars. A null value means "empty", which deletes the tag.
func (t *TagValue) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Tag == "!!null" {
			*t = TagValue{""}
			return nil
		}
		*t = TagValue{n.Value}
		return nil
	case yaml.SequenceNode:
		out := make(TagValue, 0, len(n.Content))
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: tag list items must be plain values", c.Line)
			}
			out = append(out, c.Value)
		}
		*t = out
		return nil
	}
	return fmt.Errorf("line %d: a tag value must be a plain value or a list of plain values", n.Line)
}

// MarshalYAML writes single values as scalars and lists as sequences.
func (t TagValue) MarshalYAML() (any, error) {
	if len(t) == 1 {
		return t[0], nil
	}
	return []string(t), nil
}

var (
	// [Group:[Group:]]Tag[#]  (the trailing # asks ExifTool for the raw numeric form)
	setTagRE = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z0-9_-]*:){0,2}[A-Za-z][A-Za-z0-9_-]*#?$`)
	// like setTagRE, but * and ? wildcards are allowed and "all" means everything
	removeTagRE = regexp.MustCompile(`^(?:[A-Za-z*?][A-Za-z0-9_*?-]*:){0,2}[A-Za-z*?][A-Za-z0-9_*?-]*$`)
)

// ValidateSetTag checks the syntax of a tag name used for writing, such as
// "Artist", "XMP-dc:Rights" or "Orientation#".
func ValidateSetTag(name string) error {
	if !setTagRE.MatchString(name) {
		return fmt.Errorf("invalid tag name %q (expected Tag or Group:Tag, e.g. Artist or XMP-dc:Rights)", name)
	}
	if _, tag, _ := SplitTag(name); strings.EqualFold(tag, "all") {
		return fmt.Errorf("%q cannot be set; use remove or strip to delete tags", name)
	}
	return nil
}

// ValidateRemoveTag checks the syntax of a tag name used for deletion.
// Wildcards and the special tag "all" (e.g. "XMP:all", "GPS*") are allowed.
func ValidateRemoveTag(name string) error {
	if !removeTagRE.MatchString(name) {
		return fmt.Errorf("invalid tag name %q (expected Tag, Group:Tag or Group:all, e.g. Comment or XMP:all)", name)
	}
	return nil
}

// SplitTag splits "Group:Tag#" into its parts. The group is "" when absent
// and may itself contain a colon for two-level groups.
func SplitTag(name string) (group, tag string, numeric bool) {
	if strings.HasSuffix(name, "#") {
		numeric = true
		name = strings.TrimSuffix(name, "#")
	}
	if i := strings.LastIndex(name, ":"); i >= 0 {
		return name[:i], name[i+1:], numeric
	}
	return "", name, numeric
}
