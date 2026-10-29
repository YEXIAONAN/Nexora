// Package observability provides structured logging and HTTP middleware for
// correlation, access logging, panic recovery and security headers.
package observability

import (
	"log/slog"
	"os"
)

// NewLogger builds the process-wide structured logger. Logs are emitted as JSON
// so that they can be collected and queried without a parsing pipeline.
func NewLogger(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(handler).With("service", "nexora-api")
}
