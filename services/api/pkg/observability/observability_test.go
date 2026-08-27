package observability

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseTraceParent(t *testing.T) {
	const valid = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	trace, ok := ParseTraceParent(valid)
	if !ok {
		t.Fatal("valid traceparent was rejected")
	}
	if trace.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || !trace.Sampled {
		t.Errorf("parsed %+v", trace)
	}
	if trace.TraceParent() != valid {
		t.Errorf("round trip = %q, want %q", trace.TraceParent(), valid)
	}

	invalid := []string{
		"",
		"garbage",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", // unsupported version
		"00-4bf92f3577b34da6a3ce929d0e0e473-00f067aa0ba902b7-01",  // short trace id
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01", // all-zero trace id
		"00-4bf92f3577b34da6a3ce929d0e0e4736-zzzzzzzzzzzzzzzz-01", // non-hex span id
	}
	for _, header := range invalid {
		if _, ok := ParseTraceParent(header); ok {
			t.Errorf("accepted malformed traceparent %q", header)
		}
	}
}

func TestChildKeepsTraceAndChangesSpan(t *testing.T) {
	parent := NewTrace()
	child := parent.Child()
	if child.TraceID != parent.TraceID {
		t.Error("child must continue the same trace")
	}
	if child.SpanID == parent.SpanID {
		t.Error("child must start a new span")
	}
}

// Acceptance criterion 6: structured logs carry request ID and trace context.
func TestRequestContextEmitsIdentityOnEveryLine(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelInfo, "api", "test")

	handler := Chain(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
		RequestContext(logger),
		AccessLog(),
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(HeaderTraceParent, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get(HeaderRequestID) == "" {
		t.Error("response is missing X-Request-Id")
	}

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("log line is not JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"request_id", "trace_id", "span_id", "status", "latency_ms", "service"} {
		if _, ok := line[key]; !ok {
			t.Errorf("log line is missing %q: %s", key, buf.String())
		}
	}
	if line["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("inbound trace was not continued: %v", line["trace_id"])
	}
	if line["status"] != float64(http.StatusTeapot) {
		t.Errorf("status = %v, want 418", line["status"])
	}
}

func TestRequestContextHonoursAnInboundRequestID(t *testing.T) {
	handler := Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := RequestIDFrom(r.Context()); got != "caller-supplied" {
				t.Errorf("request id = %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		}),
		RequestContext(NewLogger(&bytes.Buffer{}, slog.LevelInfo, "api", "test")),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderRequestID, "caller-supplied")
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestRecoverTurnsPanicIntoAHandledResponse(t *testing.T) {
	var buf bytes.Buffer
	called := false

	handler := Chain(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }),
		RequestContext(NewLogger(&buf, slog.LevelInfo, "api", "test")),
		Recover(func(w http.ResponseWriter, _ *http.Request, _ any) {
			called = true
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Fatal("panic handler was not invoked")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(buf.String(), "panic recovered") {
		t.Error("panic was not logged")
	}
}
