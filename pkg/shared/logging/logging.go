// Package logging configures the global slog logger used across all services.
//
// Logging lives in pkg/shared (not queen/internal) so the worker can use
// the same configuration helpers without violating Go's `internal` rule.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options controls logger construction.
type Options struct {
	// Level is one of: debug, info, warn, error. Defaults to "info".
	Level string

	// Format is one of: text, json. Defaults to "text".
	Format string
}

// Setup configures slog.Default to the chosen level and format.
// It returns the created logger so callers can also use it directly.
//
// Default behavior:
//   - LOG_LEVEL=debug enables verbose logging.
//   - LOG_FORMAT=json enables machine-readable output (great for production).
//   - Otherwise the logger writes pretty text to stderr.
func Setup(opts Options) *slog.Logger {
	level := parseLevel(opts.Level)
	format := strings.ToLower(strings.TrimSpace(opts.Format))

	var handler slog.Handler
	var writer io.Writer = os.Stderr

	switch format {
	case "json":
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level})
	default:
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
