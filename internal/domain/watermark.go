package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Position is one of nine anchor positions.
type Position string

// Anchor positions.
const (
	TopLeft      Position = "top-left"
	TopCenter    Position = "top-center"
	TopRight     Position = "top-right"
	CenterLeft   Position = "center-left"
	Center       Position = "center"
	CenterRight  Position = "center-right"
	BottomLeft   Position = "bottom-left"
	BottomCenter Position = "bottom-center"
	BottomRight  Position = "bottom-right"
)

// Positions lists every valid anchor.
var Positions = []Position{
	TopLeft, TopCenter, TopRight,
	CenterLeft, Center, CenterRight,
	BottomLeft, BottomCenter, BottomRight,
}

// ParsePosition validates a position string.
func ParsePosition(s string) (Position, error) {
	n := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "_", "-")))
	for _, p := range Positions {
		if string(p) == n {
			return p, nil
		}
	}
	names := make([]string, len(Positions))
	for i, p := range Positions {
		names[i] = string(p)
	}
	return "", fmt.Errorf("unknown position %q (valid: %s)", s, strings.Join(names, ", "))
}

// WatermarkConfig describes a transparent-image signature watermark.
type WatermarkConfig struct {
	Image    string  `yaml:"image" json:"image"`
	Position string  `yaml:"position" json:"position"`
	Opacity  float64 `yaml:"opacity" json:"opacity"`
	Scale    float64 `yaml:"scale" json:"scale"`
	Margin   int     `yaml:"margin" json:"margin"`
}

// DefaultWatermark returns the default placement settings (no image).
func DefaultWatermark() WatermarkConfig {
	return WatermarkConfig{
		Position: string(BottomRight),
		Opacity:  0.8,
		Scale:    0.12,
		Margin:   32,
	}
}

// Validate checks every field and returns all problems at once.
func (c WatermarkConfig) Validate() error {
	var errs []error
	if strings.TrimSpace(c.Image) == "" {
		errs = append(errs, Invalidf("watermark.image is required"))
	}
	if _, err := ParsePosition(c.Position); err != nil {
		errs = append(errs, Invalidf("watermark.position: %v", err))
	}
	if c.Opacity <= 0 || c.Opacity > 1 {
		errs = append(errs, Invalidf("watermark.opacity must be in (0, 1], got %v", c.Opacity))
	}
	if c.Scale <= 0 || c.Scale > 1 {
		errs = append(errs, Invalidf("watermark.scale must be in (0, 1], got %v", c.Scale))
	}
	if c.Margin < 0 {
		errs = append(errs, Invalidf("watermark.margin must be >= 0, got %d", c.Margin))
	}
	return errors.Join(errs...)
}
