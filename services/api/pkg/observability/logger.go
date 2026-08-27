// Package observability provides structured logging, request identity and W3C
// trace-context propagation. Document 25 requires every request to carry a
// request ID, structured logs, latency, status and trace context.
//
// Phase 1 emits trace context; exporting spans to a collector (OpenTelemetry,
// document 12) is wired with the rest of production observability.
package observability

import (
	"io"
	"log/slog"
)

// NewLogger returns a JSON logger. JSON in every environment, including
// development: a log line that is parseable locally is parseable in production,
// and a second format is a second thing to get wrong.
func NewLogger(w io.Writer, level slog.Level, service, version string) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				attr.Key = "timestamp"
			}
			return attr
		},
	})
	return slog.New(handler).With(
		slog.String("service", service),
		slog.String("version", version),
	)
}
