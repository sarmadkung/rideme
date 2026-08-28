package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Acceptance criterion 7: the taxonomy maps to HTTP status consistently.
func TestStatusForCoversEveryDocumentedCode(t *testing.T) {
	want := map[Code]int{
		CodeNotFound:     http.StatusNotFound,
		CodeUnauthorized: http.StatusUnauthorized,
		CodeForbidden:    http.StatusForbidden,
		CodeConflict:     http.StatusConflict,
		CodeValidation:   http.StatusUnprocessableEntity,
		CodeUnavailable:  http.StatusServiceUnavailable,
		CodeInternal:     http.StatusInternalServerError,
	}
	for code, status := range want {
		if got := StatusFor(code); got != status {
			t.Errorf("StatusFor(%q) = %d, want %d", code, got, status)
		}
	}
	if got := StatusFor(Code("invented")); got != http.StatusInternalServerError {
		t.Errorf("unknown code must map to 500, got %d", got)
	}
}

func TestErrorsIsMatchesByCode(t *testing.T) {
	err := NotFound("driver 42 does not exist").WithCause(errors.New("sql: no rows"))

	if !errors.Is(err, ErrNotFound) {
		t.Error("errors.Is should match on code")
	}
	if errors.Is(err, ErrConflict) {
		t.Error("errors.Is must not match a different code")
	}
	if unwrapped := errors.Unwrap(err); unwrapped == nil {
		t.Error("cause should remain reachable for logging")
	}
}

func TestAsErrorClassifiesUnknownErrorsAsInternal(t *testing.T) {
	classified := AsError(errors.New("something exploded in a driver"))
	if classified.Code != CodeInternal {
		t.Errorf("code = %q, want internal", classified.Code)
	}
	if classified.Message != "internal error" {
		t.Errorf("an unclassified error must not leak its message, got %q", classified.Message)
	}
}

func TestWriteErrorDoesNotLeakTheCause(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jobs/42", nil)

	WriteError(rec, req, NotFound("job not found").
		WithCause(errors.New("pq: relation \"jobs\" does not exist")))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Code != CodeNotFound || body.Message != "job not found" {
		t.Errorf("unexpected envelope: %+v", body)
	}
	if got := rec.Body.String(); strings.Contains(got, "relation") {
		t.Errorf("internal cause leaked to the client: %s", got)
	}
}

func TestValidationCarriesFieldDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs", nil)

	WriteError(rec, req, Validation("invalid request", map[string]string{
		"pickup.lat": "must be between -90 and 90",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	var body ErrorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Details["pickup.lat"] == "" {
		t.Errorf("field details were dropped: %+v", body)
	}
}

func TestNotFoundHandlerUsesThePlatformEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFoundHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("content type = %q", ct)
	}
}

func TestClampLimit(t *testing.T) {
	cases := map[int]int{
		0:    DefaultPageLimit,
		-1:   DefaultPageLimit,
		1:    1,
		25:   25,
		100:  MaxPageLimit,
		1000: MaxPageLimit,
	}
	for requested, want := range cases {
		if got := ClampLimit(requested); got != want {
			t.Errorf("ClampLimit(%d) = %d, want %d", requested, got, want)
		}
	}
}
