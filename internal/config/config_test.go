package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestRootCmd creates a cobra.Command with the same persistent flags as the
// real root command so that Load can bind them during tests.
func newTestRootCmd() *cobra.Command {
	cmd := &cobra.Command{}
	pf := cmd.PersistentFlags()
	pf.String("config", "", "")
	pf.String("log-level", "info", "")
	pf.String("log-format", "text", "")
	pf.Bool("no-color", false, "")
	pf.BoolP("quiet", "q", false, "")

	return cmd
}

// writeTempConfig writes a YAML string to a temporary file and returns the path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))

	return p
}

// ---------------------------------------------------------------------------
// Default
// ---------------------------------------------------------------------------

func TestDefault(t *testing.T) {
	cfg := Default()
	assert.Equal(t, LogLevelInfo, cfg.LogLevel)
	assert.Equal(t, LogFormatText, cfg.LogFormat)
	assert.False(t, cfg.NoColor)
	assert.False(t, cfg.Quiet)
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

func TestValidate_ValidValues(t *testing.T) {
	for _, lvl := range []string{"debug", "info", "warn", "error"} {
		cfg := Default()
		cfg.LogLevel = lvl
		assert.NoError(t, cfg.Validate(), "level=%s", lvl)
	}

	for _, fmt := range []string{"text", "json"} {
		cfg := Default()
		cfg.LogFormat = fmt
		assert.NoError(t, cfg.Validate(), "format=%s", fmt)
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := Default()
	cfg.LogLevel = "verbose"
	assert.ErrorContains(t, cfg.Validate(), "invalid log level")
}

func TestValidate_InvalidLogFormat(t *testing.T) {
	cfg := Default()
	cfg.LogFormat = "xml"
	assert.ErrorContains(t, cfg.Validate(), "invalid log format")
}

// ---------------------------------------------------------------------------
// EffectiveLogLevel
// ---------------------------------------------------------------------------

func TestEffectiveLogLevel_Normal(t *testing.T) {
	cfg := &Config{LogLevel: "debug"}
	assert.Equal(t, "debug", cfg.EffectiveLogLevel())
}

func TestEffectiveLogLevel_QuietOverride(t *testing.T) {
	cfg := &Config{LogLevel: "debug", Quiet: true}
	assert.Equal(t, "error", cfg.EffectiveLogLevel())
}

// ---------------------------------------------------------------------------
// Load — defaults only
// ---------------------------------------------------------------------------

func TestLoad_DefaultsOnly(t *testing.T) {
	cfg, err := Load(nil, "")
	require.NoError(t, err)
	assert.Equal(t, LogLevelInfo, cfg.LogLevel)
	assert.Equal(t, LogFormatText, cfg.LogFormat)
	assert.False(t, cfg.NoColor)
	assert.False(t, cfg.Quiet)
}

// ---------------------------------------------------------------------------
// Load — environment variables
// ---------------------------------------------------------------------------

func TestLoad_EnvOverridesDefault(t *testing.T) {
	t.Setenv("CHART2KRO_LOG_LEVEL", "debug")

	cfg, err := Load(nil, "")
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_EnvBooleans(t *testing.T) {
	t.Setenv("CHART2KRO_NO_COLOR", "true")
	t.Setenv("CHART2KRO_QUIET", "true")

	cfg, err := Load(nil, "")
	require.NoError(t, err)
	assert.True(t, cfg.NoColor)
	assert.True(t, cfg.Quiet)
}

// ---------------------------------------------------------------------------
// Load — config file
// ---------------------------------------------------------------------------

func TestLoad_ConfigFile(t *testing.T) {
	p := writeTempConfig(t, "log-level: warn\nlog-format: json\n")

	cfg, err := Load(nil, p)
	require.NoError(t, err)
	assert.Equal(t, "warn", cfg.LogLevel)
	assert.Equal(t, "json", cfg.LogFormat)
}

func TestLoad_MissingExplicitFile(t *testing.T) {
	_, err := Load(nil, "/tmp/nonexistent-chart2kro-cfg-12345.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoad_MalformedFile(t *testing.T) {
	p := writeTempConfig(t, ": invalid yaml :")

	_, err := Load(nil, p)
	require.Error(t, err)
}

func TestLoad_MissingAutoDiscoverFile(t *testing.T) {
	// When no explicit file is given and auto-discover finds nothing, Load
	// should succeed with defaults.
	cfg, err := Load(nil, "")
	require.NoError(t, err)
	assert.Equal(t, LogLevelInfo, cfg.LogLevel)
}

// ---------------------------------------------------------------------------
// Load — flag precedence
// ---------------------------------------------------------------------------

func TestLoad_FlagOverridesDefault(t *testing.T) {
	cmd := newTestRootCmd()
	require.NoError(t, cmd.PersistentFlags().Set("log-level", "error"))

	cfg, err := Load(cmd, "")
	require.NoError(t, err)
	assert.Equal(t, "error", cfg.LogLevel)
}

func TestLoad_FlagOverridesEnv(t *testing.T) {
	t.Setenv("CHART2KRO_LOG_LEVEL", "debug")

	cmd := newTestRootCmd()
	require.NoError(t, cmd.PersistentFlags().Set("log-level", "error"))

	cfg, err := Load(cmd, "")
	require.NoError(t, err)
	assert.Equal(t, "error", cfg.LogLevel)
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	t.Setenv("CHART2KRO_LOG_LEVEL", "debug")
	p := writeTempConfig(t, "log-level: warn\n")

	cfg, err := Load(nil, p)
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_FlagOverridesAll(t *testing.T) {
	t.Setenv("CHART2KRO_LOG_LEVEL", "debug")
	p := writeTempConfig(t, "log-level: warn\n")

	cmd := newTestRootCmd()
	require.NoError(t, cmd.PersistentFlags().Set("log-level", "error"))

	cfg, err := Load(cmd, p)
	require.NoError(t, err)
	assert.Equal(t, "error", cfg.LogLevel)
}

// ---------------------------------------------------------------------------
// Load — validation on loaded values
// ---------------------------------------------------------------------------

func TestLoad_InvalidLogLevelFromEnv(t *testing.T) {
	t.Setenv("CHART2KRO_LOG_LEVEL", "verbose")

	_, err := Load(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestLoad_InvalidLogFormatFromFile(t *testing.T) {
	p := writeTempConfig(t, "log-format: xml\n")

	_, err := Load(nil, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log format")
}

// ---------------------------------------------------------------------------
// Context helpers
// ---------------------------------------------------------------------------

func TestContext_RoundTrip(t *testing.T) {
	cfg := &Config{LogLevel: "debug", LogFormat: "json"}
	ctx := NewContext(context.Background(), cfg)
	got := FromContext(ctx)
	assert.Equal(t, cfg, got)
}

func TestFromContext_FallbackToDefault(t *testing.T) {
	got := FromContext(context.Background())
	assert.Equal(t, Default(), got)
}
