// Package config loads PhotoKit's optional user configuration with Viper.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Defaults.
const (
	DefaultJobs   = 4
	DefaultOutput = "./export"
	MaxJobs       = 64
)

// Config holds settings that can come from a file or the environment.
type Config struct {
	ExifToolPath string   // exiftool.path / PHOTOKIT_EXIFTOOL_PATH
	Jobs         int      // jobs / PHOTOKIT_JOBS
	Output       string   // output / PHOTOKIT_OUTPUT
	PresetDirs   []string // preset_dirs
	File         string   // config file that was used, if any
}

// UserConfigDir returns <user config dir>/photokit.
func UserConfigDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "photokit")
}

// Load reads configuration. explicit is the --config file (must exist);
// otherwise photokit.{yaml,yml,json,toml} is searched in the working
// directory and the user config directory. Environment variables use the
// PHOTOKIT_ prefix.
func Load(explicit string) (*Config, error) {
	v := viper.New()
	v.SetDefault("jobs", DefaultJobs)
	v.SetDefault("output", DefaultOutput)

	v.SetEnvPrefix("PHOTOKIT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if explicit != "" {
		v.SetConfigFile(explicit)
	} else {
		v.SetConfigName("photokit")
		v.AddConfigPath(".")
		if d := UserConfigDir(); d != "" {
			v.AddConfigPath(d)
		}
	}
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if explicit != "" || !errors.As(err, &notFound) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	c := &Config{
		ExifToolPath: v.GetString("exiftool.path"),
		Jobs:         v.GetInt("jobs"),
		Output:       v.GetString("output"),
		PresetDirs:   v.GetStringSlice("preset_dirs"),
		File:         v.ConfigFileUsed(),
	}
	if c.Jobs < 1 || c.Jobs > MaxJobs {
		return nil, fmt.Errorf("invalid jobs value %d (must be 1-%d)", c.Jobs, MaxJobs)
	}
	if c.Output == "" {
		c.Output = DefaultOutput
	}
	return c, nil
}

// SearchDirs returns the directories searched for presets by name.
func (c *Config) SearchDirs() []string {
	dirs := append([]string{"presets"}, c.PresetDirs...)
	if d := UserConfigDir(); d != "" {
		dirs = append(dirs, filepath.Join(d, "presets"))
	}
	return dirs
}
