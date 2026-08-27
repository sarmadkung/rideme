package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
)

type contextKey int

const (
	requestIDKey contextKey = iota
	traceKey
	loggerKey
)

// HeaderRequestID is echoed on every response so a client can quote it in a
// support ticket and an operator can find the exact request.
const HeaderRequestID = "X-Request-Id"

// HeaderTraceParent is the W3C Trace Context header.
const HeaderTraceParent = "traceparent"

// Trace is the subset of W3C trace context the platform propagates.
type Trace struct {
	TraceID string // 32 lowercase hex characters
	SpanID  string // 16 lowercase hex characters
	Sampled bool
}

func (t Trace) TraceParent() string {
	flags := "00"
	if t.Sampled {
		flags = "01"
	}
	return "00-" + t.TraceID + "-" + t.SpanID + "-" + flags
}

// ParseTraceParent accepts an inbound traceparent header. An absent or
// malformed header is not an error — a new trace is started instead, because
// refusing a request over a bad header would let a caller break the API.
func ParseTraceParent(header string) (Trace, bool) {
	parts := strings.Split(strings.TrimSpace(header), "-")
	if len(parts) != 4 || parts[0] != "00" {
		return Trace{}, false
	}
	traceID, spanID, flags := parts[1], parts[2], parts[3]
	if !isHex(traceID, 32) || !isHex(spanID, 16) || !isHex(flags, 2) {
		return Trace{}, false
	}
	if traceID == strings.Repeat("0", 32) || spanID == strings.Repeat("0", 16) {
		return Trace{}, false
	}
	return Trace{TraceID: traceID, SpanID: spanID, Sampled: flags != "00"}, true
}

// NewTrace starts a root trace.
func NewTrace() Trace {
	return Trace{TraceID: randomHex(16), SpanID: randomHex(8), Sampled: true}
}

// Child keeps the trace ID and starts a new span, which is what an inbound
// request becomes when it continues someone else's trace.
func (t Trace) Child() Trace {
	return Trace{TraceID: t.TraceID, SpanID: randomHex(8), Sampled: t.Sampled}
}

func isHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func randomHex(bytes int) string {
	buf := make([]byte, bytes)
	// crypto/rand.Read never returns an error on supported platforms.
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// NewRequestID returns an opaque, collision-resistant request identifier.
func NewRequestID() string { return randomHex(16) }

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func WithTrace(ctx context.Context, t Trace) context.Context {
	return context.WithValue(ctx, traceKey, t)
}

func TraceFrom(ctx context.Context) (Trace, bool) {
	t, ok := ctx.Value(traceKey).(Trace)
	return t, ok
}

// WithLogger stores a request-scoped logger. Handlers use LoggerFrom rather
// than a package-level logger so every line carries request and trace identity.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// LoggerFrom never returns nil; a caller outside a request gets the default.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}
