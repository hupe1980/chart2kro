// Package config provides configuration management for chart2kro.
//
// Configuration is loaded from three sources with the following precedence
// (highest to lowest):
//  1. CLI flags
//  2. Environment variables (CHART2KRO_ prefix)
//  3. Config file (.chart2kro.yaml)
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Supported log levels.
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// Supported log formats.
const (
	LogFormatText = "text"
	LogFormatJSON = "json"
)

// Config represents the global configuration for chart2kro.
type Config struct {
	// LogLevel controls the verbosity of log output.
	// Valid values: debug, info, warn, error.
	LogLevel string `mapstructure:"log-level" json:"logLevel"`

	// LogFormat controls the format of log output.
	// Valid values: text, json.
	LogFormat string `mapstructure:"log-format" json:"logFormat"`

	// NoColor disables colored output.
	NoColor bool `mapstructure:"no-color" json:"noColor"`

	// Quiet suppresses all log output below error level.
	Quiet bool `mapstructure:"quiet" json:"quiet"`

	// ConfigFile is the resolved path to the config file used.
	// Set after Load() — not read from config itself.
	ConfigFile string `mapstructure:"-" json:"-"`
}

// Default returns a Config with sensible default values.
func Default() *Config {
	return &Config{
		LogLevel:  LogLevelInfo,
		LogFormat: LogFormatText,
		NoColor:   false,
		Quiet:     false,
	}
}

// Validate checks that all config values are valid.
func (c *Config) Validate() error {
	switch c.LogLevel {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		// valid
	default:
		return fmt.Errorf("invalid log level %q: must be one of debug, info, warn, error", c.LogLevel)
	}

	switch c.LogFormat {
	case LogFormatText, LogFormatJSON:
		// valid
	default:
		return fmt.Errorf("invalid log format %q: must be one of text, json", c.LogFormat)
	}

	return nil
}

// EffectiveLogLevel returns the log level to use. When Quiet is true the log
// level is overridden to "error" regardless of the configured LogLevel.
func (c *Config) EffectiveLogLevel() string {
	if c.Quiet {
		return LogLevelError
	}

	return c.LogLevel
}

// Load initialises configuration from flags, environment variables, and an
// optional config file. A fresh viper instance is used on every call so that
// Load is safe for concurrent tests.
func Load(cmd *cobra.Command, configFile string) (*Config, error) {
	v := viper.New()

	setDefaults(v)
	configureEnv(v)

	if err := configureFile(v, configFile); err != nil {
		return nil, err
	}

	if err := bindFlags(v, cmd); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Store the resolved config file path so downstream code can locate it.
	cfg.ConfigFile = v.ConfigFileUsed()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// setDefaults registers default values in viper.
func setDefaults(v *viper.Viper) {
	v.SetDefault("log-level", LogLevelInfo)
	v.SetDefault("log-format", LogFormatText)
	v.SetDefault("no-color", false)
	v.SetDefault("quiet", false)
}

// configureEnv sets up environment variable support.
func configureEnv(v *viper.Viper) {
	v.SetEnvPrefix("CHART2KRO")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
}

// configureFile sets up the config file source.
func configureFile(v *viper.Viper, configFile string) error {
	if configFile != "" {
		v.SetConfigFile(configFile)

		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("reading config file %q: %w", configFile, err)
		}

		return nil
	}

	// Auto-discovery mode.
	v.SetConfigName(".chart2kro")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ".config", "chart2kro"))
	}

	if err := v.ReadInConfig(); err != nil {
		// No config file found → perfectly fine in auto-discovery.
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}

		// Found a file but it was malformed.
		return fmt.Errorf("parsing config file: %w", err)
	}

	return nil
}

// bindFlags walks from cmd up to the root and binds all PersistentFlags.
func bindFlags(v *viper.Viper, cmd *cobra.Command) error {
	if cmd == nil {
		return nil
	}

	// Bind the current command's own flags.
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("binding flags: %w", err)
	}

	// Walk up to root and bind all persistent flags at each level.
	for c := cmd; c != nil; c = c.Parent() {
		if err := v.BindPFlags(c.PersistentFlags()); err != nil {
			return fmt.Errorf("binding persistent flags: %w", err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Context helpers
// ---------------------------------------------------------------------------

type ctxKey struct{}
type ctxFileKey struct{}

// NewContext returns a child context carrying cfg.
func NewContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, ctxKey{}, cfg)
}

// FromContext extracts a Config from ctx, falling back to Default().
func FromContext(ctx context.Context) *Config {
	if cfg, ok := ctx.Value(ctxKey{}).(*Config); ok {
		return cfg
	}

	return Default()
}

// NewContextWithConfigFile returns a child context carrying the resolved
// config file path. This allows downstream code to locate the config file
// without re-discovering it.
func NewContextWithConfigFile(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, ctxFileKey{}, path)
}

// ConfigFileFromContext extracts the config file path from ctx.
// Returns empty string if no config file was resolved.
func ConfigFileFromContext(ctx context.Context) string {
	if p, ok := ctx.Value(ctxFileKey{}).(string); ok {
		return p
	}

	return ""
}
