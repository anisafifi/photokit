package cli

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"photokit/internal/preset"
)

func newPresetCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preset",
		Short: "List, inspect and validate presets",
	}
	cmd.AddCommand(newPresetListCmd(e), newPresetShowCmd(e), newPresetValidateCmd(e))
	return cmd
}

func newPresetListCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "list [dir...]",
		Short: "List presets (default: ./presets and the configured preset directories)",
		RunE: func(cmd *cobra.Command, args []string) error {
			dirs := args
			if len(dirs) == 0 {
				dirs = e.cfg.SearchDirs()
			}
			files := preset.List(dirs)
			if len(files) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No presets found in: %s\nCreate one with 'photokit init'.\n", strings.Join(dirs, ", "))
				return nil
			}
			t := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(t, "NAME\tSTATUS\tFILE\tDESCRIPTION")
			for _, f := range files {
				p, err := preset.LoadUnvalidated(f)
				name, desc, status := "-", "", "ok"
				if err != nil {
					status = "invalid"
				} else {
					if p.Name != "" {
						name = p.Name
					}
					desc = p.Description
					if p.Validate() != nil {
						status = "invalid"
					}
				}
				fmt.Fprintf(t, "%s\t%s\t%s\t%s\n", name, status, f, desc)
			}
			return t.Flush()
		},
	}
}

func newPresetShowCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "show <preset>",
		Short: "Print a preset with paths resolved",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := preset.Find(args[0], e.cfg.SearchDirs())
			if err != nil {
				return err
			}
			p, err := preset.Load(path)
			if err != nil {
				return err
			}
			data, err := yaml.Marshal(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "# %s (relative asset paths resolved)\n%s", path, data)
			return nil
		},
	}
}

func newPresetValidateCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <preset>...",
		Short: "Check presets for unknown fields, bad values and missing assets",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			bad := 0
			for _, a := range args {
				path, err := preset.Find(a, e.cfg.SearchDirs())
				if err == nil {
					var p *preset.Preset
					if p, err = preset.LoadUnvalidated(path); err == nil {
						err = p.Validate()
					}
				}
				if err != nil {
					bad++
					fmt.Fprintf(w, "INVALID  %s\n", a)
					for _, line := range flatten(err) {
						fmt.Fprintf(w, "  - %s\n", line)
					}
					continue
				}
				fmt.Fprintf(w, "OK       %s\n", path)
			}
			if bad > 0 {
				return fmt.Errorf("%d preset(s) invalid", bad)
			}
			return nil
		},
	}
}

// flatten splits joined errors into one message per line.
func flatten(err error) []string {
	var out []string
	var j interface{ Unwrap() []error }
	if errors.As(err, &j) {
		for _, e := range j.Unwrap() {
			out = append(out, flatten(e)...)
		}
		return out
	}
	for _, l := range strings.Split(err.Error(), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
