package preset

import (
	"errors"
	"os"

	"photokit/internal/domain"
)

// Validate checks the whole preset and reports every problem at once.
func (p *Preset) Validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, domain.Invalidf(format, args...)) }

	if p.Version != CurrentVersion {
		bad("version must be %d (got %d)", CurrentVersion, p.Version)
	}

	if err := p.Metadata.Validate(); err != nil {
		errs = append(errs, err)
	}

	if p.Watermark != nil {
		if cfg := p.WatermarkConfig(); cfg != nil {
			if err := cfg.Validate(); err != nil {
				errs = append(errs, err)
			}
			// Check the file independently so every problem is reported at once.
			if cfg.Image != "" {
				if st, err := os.Stat(cfg.Image); err != nil {
					bad("watermark.image: cannot read %s", cfg.Image)
				} else if st.IsDir() {
					bad("watermark.image: %s is a directory", cfg.Image)
				}
			}
		}
	}

	if _, err := domain.ParseExportFormat(p.Export.Format); err != nil {
		errs = append(errs, err)
	}
	if q := p.Export.Quality; q != 0 && (q < 1 || q > 100) {
		bad("export.quality must be between 1 and 100 (got %d)", q)
	}
	if p.Export.PreserveOriginal != nil && !*p.Export.PreserveOriginal {
		bad("export.preserve_original cannot be false: presets never modify source images")
	}
	return errors.Join(errs...)
}
