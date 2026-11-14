// Package cli wires Cobra commands to the application services. Commands
// parse flags and print results; they contain no processing logic.
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"photokit/internal/config"
	"photokit/internal/domain"
	"photokit/internal/metadata"
	"photokit/internal/version"
)

// env carries state shared by all commands for one invocation.
type env struct {
	stdout, stderr io.Writer

	cfgFile      string
	exiftoolFlag string
	verbose      bool

	cfg    *config.Config
	log    *slog.Logger
	engine metadata.Engine
}

// Engine lazily detects ExifTool; commands that do not need it never pay for
// (or fail on) its absence.
func (e *env) Engine(ctx context.Context) (metadata.Engine, error) {
	if e.engine != nil {
		return e.engine, nil
	}
	path := e.exiftoolFlag
	if path == "" {
		path = e.cfg.ExifToolPath
	}
	et, err := metadata.Detect(ctx, path)
	if err != nil {
		return nil, err
	}
	e.log.Debug("exiftool detected", "path", et.Path, "version", et.Version)
	e.engine = et
	return et, nil
}

// jobs resolves the --jobs flag against the configured default.
func (e *env) jobs(flag int) (int, error) {
	if flag == 0 {
		return e.cfg.Jobs, nil
	}
	if flag < 1 || flag > config.MaxJobs {
		return 0, domain.Invalidf("--jobs must be between 1 and %d, got %d", config.MaxJobs, flag)
	}
	return flag, nil
}

func (e *env) setup() error {
	cfg, err := config.Load(e.cfgFile)
	if err != nil {
		return err
	}
	e.cfg = cfg
	level := slog.LevelWarn
	if e.verbose {
		level = slog.LevelDebug
	}
	e.log = slog.New(slog.NewTextHandler(e.stderr, &slog.HandlerOptions{Level: level}))
	if cfg.File != "" {
		e.log.Debug("config loaded", "file", cfg.File)
	}
	return nil
}

func newRoot(e *env) *cobra.Command {
	root := &cobra.Command{
		Use:   "photokit",
		Short: "Photography automation: metadata, copyright, signatures and batch export",
		Long: `PhotoKit automates repetitive photography production tasks:

  - inspect and edit EXIF / IPTC / XMP metadata (creator, copyright, keywords…)
  - stamp a transparent PNG signature on JPEG and PNG images
  - run whole workflows from reusable YAML presets, in parallel

PhotoKit is non-destructive: sources are only ever read. Results are written
as copies into an output directory (default ./export) via atomic renames.

Metadata is handled by ExifTool, which must be installed separately.`,
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return e.setup()
		},
	}
	root.SetOut(e.stdout)
	root.SetErr(e.stderr)
	root.SetVersionTemplate("photokit {{.Version}}\n")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return fmt.Errorf("%w\nRun '%s --help' for usage", err, cmd.CommandPath())
	})

	pf := root.PersistentFlags()
	pf.StringVar(&e.cfgFile, "config", "", "config file (default: ./photokit.yaml or the user config dir)")
	pf.StringVar(&e.exiftoolFlag, "exiftool", "", "path to the exiftool executable (default: search PATH)")
	pf.BoolVarP(&e.verbose, "verbose", "v", false, "show detailed processing logs on stderr")

	root.AddCommand(
		newInitCmd(e),
		newVersionCmd(e),
		newInfoCmd(e),
		newMetadataCmd(e),
		newWatermarkCmd(e),
		newProcessCmd(e),
		newPresetCmd(e),
	)
	return root
}

// Run executes photokit with args and returns the process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	root := newRoot(e)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}

// Execute runs photokit against the real standard streams.
func Execute(ctx context.Context, args []string) int {
	return Run(ctx, args, os.Stdout, os.Stderr)
}
