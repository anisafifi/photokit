package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"photokit/internal/metadata"
	"photokit/internal/preset"
)

const presetTemplate = `# PhotoKit preset. Check it with: photokit preset validate {{.File}}
version: 1
name: default
description: Standard photography export

# Written to EXIF, IPTC and XMP. Remove a line to leave that field untouched.
metadata:
  creator: {{.Creator}}
  copyright: {{.Copyright}}
  rights: All rights reserved
  keywords:
    - photography

# Transparent PNG signature. Relative paths are resolved against this file.
# Set enabled: false (or remove the section) to skip the signature.
watermark:
  enabled: true
  image: ../assets/signature.png
  position: bottom-right   # top-left | top-center | top-right | center-left | center
                           # center-right | bottom-left | bottom-center | bottom-right
  opacity: 0.8             # 0-1
  scale: 0.12              # signature width relative to the image width
  margin: 32               # pixels from the edges

# Output. Originals are never modified; results are written as copies.
export:
  preserve_original: true  # must stay true
  format: original         # original | jpeg | png
  quality: 92              # JPEG quality, 1-100
  directory: ./export      # relative to where you run photokit
  # suffix: -signed        # appended to file names, before the extension
`

const configTemplate = `# PhotoKit configuration (all values are optional).
# Environment variables work too: PHOTOKIT_JOBS, PHOTOKIT_OUTPUT, PHOTOKIT_EXIFTOOL_PATH.

# Files processed in parallel.
jobs: 4

# Default output directory for commands that write copies.
output: ./export

# Extra directories searched for presets given by name (./presets is always searched).
# preset_dirs:
#   - ~/photo-presets

# exiftool:
#   path: /usr/local/bin/exiftool
`

func newInitCmd(e *env) *cobra.Command {
	var (
		dir       string
		creator   string
		copyright string
		force     bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter project: config, default preset and signature placeholder",
		Long: `Create, in the target directory:

  photokit.yaml           optional configuration
  presets/default.yaml    a ready-to-edit preset
  assets/signature.png    a placeholder signature (replace it with your own)

Existing files are never overwritten unless you pass --force.`,
		Example: `  photokit init
  photokit init --creator "Jane Doe" --copyright "© 2026 Jane Doe"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			name := creator
			if name == "" {
				name = "Your Name"
			}
			if copyright == "" {
				copyright = fmt.Sprintf("© %d %s", time.Now().Year(), name)
			}

			presetPath := filepath.Join(dir, "presets", "default.yaml")
			var sb strings.Builder
			err := template.Must(template.New("preset").Parse(presetTemplate)).Execute(&sb, map[string]string{
				"Creator": yamlScalar(name), "Copyright": yamlScalar(copyright), "File": presetPath,
			})
			if err != nil {
				return err
			}

			files := []struct{ path, content string }{
				{filepath.Join(dir, "photokit.yaml"), configTemplate},
				{presetPath, sb.String()},
			}
			if err := os.MkdirAll(filepath.Join(dir, "presets"), 0o755); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
				return err
			}
			for _, f := range files {
				status, err := writeNew(f.path, []byte(f.content), force)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "%-8s %s\n", status, f.path)
			}

			sig := filepath.Join(dir, "assets", "signature.png")
			if _, err := os.Stat(sig); err == nil && !force {
				fmt.Fprintf(w, "%-8s %s\n", "exists", sig)
			} else {
				if force {
					_ = os.Remove(sig)
				}
				if err := writePlaceholderSignature(sig, name); err != nil {
					return err
				}
				fmt.Fprintf(w, "%-8s %s (placeholder: replace it with your own transparent PNG)\n", "created", sig)
			}

			if _, err := preset.Load(presetPath); err != nil {
				fmt.Fprintf(w, "\nNote: %v\n", err)
			}

			fmt.Fprintln(w)
			if et, err := metadata.Detect(cmd.Context(), firstNonEmpty(e.exiftoolFlag, e.cfg.ExifToolPath)); err != nil {
				fmt.Fprintf(w, "WARNING: ExifTool is not available yet.\n%v\n\n", err)
			} else {
				fmt.Fprintf(w, "ExifTool %s found at %s.\n\n", et.Version, et.Path)
			}
			fmt.Fprintf(w, "Next steps:\n  photokit process ./photos --preset %s --dry-run\n  photokit process ./photos --preset %s\n", presetPath, presetPath)
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&dir, "dir", ".", "directory to initialise")
	fl.StringVar(&creator, "creator", "", "photographer name for the preset (default: Your Name)")
	fl.StringVar(&copyright, "copyright", "", `copyright notice (default: "© <year> <creator>")`)
	fl.BoolVar(&force, "force", false, "overwrite existing files")
	return cmd
}

func writeNew(path string, content []byte, force bool) (string, error) {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	status := "created"
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if _, err := os.Stat(path); err == nil {
			status = "replaced"
		}
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "exists", nil
		}
		return "", err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return "", err
	}
	return status, f.Close()
}

func yamlScalar(s string) string {
	b, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return strings.TrimSpace(string(b))
}
