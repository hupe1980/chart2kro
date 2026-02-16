// Package logging initialises a [log/slog] logger from the application
// configuration and provides context-based logger propagation.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/hupe1980/chart2kro/internal/config"
)

type ctxKey struct{}

// Setup creates a *slog.Logger configured according to cfg, writing to stderr,
// and installs it as the process-wide default via slog.SetDefault.
func Setup(cfg *config.Config) *slog.Logger {
	return SetupWithWriter(cfg, os.Stderr)
}

// SetupWithWriter creates a *slog.Logger configured according to cfg, writing
// to w, and installs it as the process-wide default via slog.SetDefault.
// Use this variant in tests to capture or suppress log output.
func SetupWithWriter(cfg *config.Config, w io.Writer) *slog.Logger {
	level := ParseLevel(cfg.EffectiveLogLevel())
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler

	switch cfg.LogFormat {
	case config.LogFormatJSON:
		handler = slog.NewJSONHandler(w, opts)
	default: // text
		handler = slog.NewTextHandler(w, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

// ParseLevel converts a string log level to slog.Level.
func ParseLevel(level string) slog.Level {
	switch level {
	case config.LogLevelDebug:
		return slog.LevelDebug
	case config.LogLevelWarn:
		return slog.LevelWarn
	case config.LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewContext returns a child context carrying logger.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext extracts a logger from ctx, falling back to slog.Default().
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}

	return slog.Default()
}
