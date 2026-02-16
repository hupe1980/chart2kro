package logging

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/hupe1980/chart2kro/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetup_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{LogLevel: "debug", LogFormat: "text"}

	logger := SetupWithWriter(cfg, &buf)
	require.NotNil(t, logger)

	logger.Info("hello")
	assert.Contains(t, buf.String(), "hello")
}

func TestSetup_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{LogLevel: "info", LogFormat: "json"}

	logger := SetupWithWriter(cfg, &buf)
	require.NotNil(t, logger)

	logger.Info("test-msg")
	assert.Contains(t, buf.String(), `"msg":"test-msg"`)
}

func TestSetup_SetsDefault(t *testing.T) {
	cfg := &config.Config{LogLevel: "info", LogFormat: "text"}
	logger := Setup(cfg)
	assert.Equal(t, logger.Handler(), slog.Default().Handler())
}

func TestSetup_QuietSuppressesInfo(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{LogLevel: "info", LogFormat: "text", Quiet: true}

	logger := SetupWithWriter(cfg, &buf)
	logger.Info("should-not-appear")
	logger.Error("should-appear")

	assert.NotContains(t, buf.String(), "should-not-appear")
	assert.Contains(t, buf.String(), "should-appear")
}

func TestSetup_DebugLevelShowsDebugMessages(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{LogLevel: "debug", LogFormat: "text"}

	logger := SetupWithWriter(cfg, &buf)
	logger.Debug("debug-msg")

	assert.Contains(t, buf.String(), "debug-msg")
}

func TestSetup_InfoLevelHidesDebugMessages(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{LogLevel: "info", LogFormat: "text"}

	logger := SetupWithWriter(cfg, &buf)
	logger.Debug("debug-hidden")

	assert.NotContains(t, buf.String(), "debug-hidden")
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseLevel(tt.input))
		})
	}
}

func TestContext_RoundTrip(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := NewContext(context.Background(), logger)
	got := FromContext(ctx)
	assert.Equal(t, logger, got)
}

func TestFromContext_FallbackToDefault(t *testing.T) {
	got := FromContext(context.Background())
	assert.Equal(t, slog.Default(), got)
}
