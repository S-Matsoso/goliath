// Package logger constructs the application's structured logger.
package logger

import (
	"log/slog"
	"os"
)

// New returns a slog logger writing to stdout. Production environments get
// JSON output for machine ingestion; everything else gets human-readable text.
func New(env string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if env == "production" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
