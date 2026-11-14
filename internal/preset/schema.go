// Package preset loads, validates and resolves YAML workflow presets.
package preset

import "photokit/internal/domain"

// CurrentVersion is the only supported schema version.
const CurrentVersion = 1

// DefaultQuality is the JPEG quality used when a preset does not set one.
const DefaultQuality = 92

// Preset is the YAML document describing a reusable workflow.
type Preset struct {
	Version     int             `yaml:"version"`
	Name        string          `yaml:"name,omitempty"`
	Description string          `yaml:"description,omitempty"`
	Metadata    domain.Metadata `yaml:"metadata,omitempty"`
	Watermark   *Watermark      `yaml:"watermark,omitempty"`
	Export      Export          `yaml:"export,omitempty"`

	// Path is the file the preset was loaded from (not part of the schema).
	Path string `yaml:"-"`
}

// Watermark is the preset's signature section.
type Watermark struct {
	Enabled  *bool   `yaml:"enabled,omitempty"`
	Image    string  `yaml:"image,omitempty"`
	Position string  `yaml:"position,omitempty"`
	Opacity  float64 `yaml:"opacity,omitempty"`
	Scale    float64 `yaml:"scale,omitempty"`
	Margin   *int    `yaml:"margin,omitempty"`
}

// Export is the preset's output section.
type Export struct {
	PreserveOriginal *bool  `yaml:"preserve_original,omitempty"`
	Format           string `yaml:"format,omitempty"`
	Quality          int    `yaml:"quality,omitempty"`
	Directory        string `yaml:"directory,omitempty"`
	Suffix           string `yaml:"suffix,omitempty"`
}

// WatermarkEnabled reports whether a watermark section exists and is on.
func (p *Preset) WatermarkEnabled() bool {
	if p.Watermark == nil {
		return false
	}
	return p.Watermark.Enabled == nil || *p.Watermark.Enabled
}

// WatermarkConfig returns the effective watermark settings (defaults filled
// in, image path already resolved) or nil when no watermark is configured.
func (p *Preset) WatermarkConfig() *domain.WatermarkConfig {
	if !p.WatermarkEnabled() {
		return nil
	}
	cfg := domain.DefaultWatermark()
	w := p.Watermark
	cfg.Image = w.Image
	if w.Position != "" {
		cfg.Position = w.Position
	}
	if w.Opacity != 0 {
		cfg.Opacity = w.Opacity
	}
	if w.Scale != 0 {
		cfg.Scale = w.Scale
	}
	if w.Margin != nil {
		cfg.Margin = *w.Margin
	}
	return &cfg
}

// ExportFormat returns the parsed export format (original on bad input; call
// Validate first).
func (p *Preset) ExportFormat() domain.ExportFormat {
	f, err := domain.ParseExportFormat(p.Export.Format)
	if err != nil {
		return domain.ExportOriginal
	}
	return f
}

// QualityOrDefault returns the JPEG quality to use.
func (p *Preset) QualityOrDefault() int {
	if p.Export.Quality == 0 {
		return DefaultQuality
	}
	return p.Export.Quality
}
