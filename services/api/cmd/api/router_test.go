package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/pkg/authn"
	"github.com/sarmadkung/rideme/services/api/pkg/health"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/observability"
)

func testRouter(t *testing.T, probeErr error) http.Handler {
	t.Helper()
	logger := observability.NewLogger(&bytes.Buffer{}, slog.LevelError, "api", "test")
	checker := health.NewChecker("api", "test", []health.Check{
		{Name: "postgres", Critical: true, Probe: func(context.Context) error { return probeErr }},
	})
	// Identity needs a database; these tests exercise health and the error
	// envelope only, so the handler is wired with a nil service and never
	// called. The issuer is real, so the authenticated routes still reject
	// unauthenticated callers correctly.
	issuer, err := authn.NewIssuer("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	// One nil per handler parameter. Positional nils are exactly how this
	// call silently falls behind the signature, so they are grouped and named
	// rather than run together: a miscount is a compile error that says
	// "not enough arguments" and nothing about which one is missing.
	return newRouter(
		checker, identity.NewHandler(nil),
		nil, nil, nil, nil, // booking, driver, zones, places
		nil, nil, nil, nil, // merchant, grocery, offers, tracking
		nil, nil, nil, // realtime, notify, credit
		nil, nil, // payment methods, payment webhooks
		issuer, "api", "test", logger, nil)
}

// Every handler the router takes is reachable through it.
//
// This test exists because the call above fell three arguments behind the
// signature and nothing said so until a compiler did. A positional list of
// nils is exactly the shape that drifts: a new handler is added to `newRouter`
// and to `main.go`, and the test keeps compiling right up until it does not.
//
// Counting the routes a fully-wired router registers would need every handler
// constructed. Asserting that a nil handler is *skipped* rather than
// dereferenced is cheap, and it is the property the nil-guards in `newRouter`
// exist to provide: a process missing an optional dependency must still serve
// the rest.
func TestARouterWithNoHandlersStillServesHealth(t *testing.T) {
	router := testRouter(t, nil)

	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("liveness returned %d with no handlers wired, want 200", recorder.Code)
	}

	// And a route whose handler is absent is a 404 rather than a panic.
	missing := httptest.NewRequest(http.MethodGet, "/api/v1/payment-methods", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, missing)

	if recorder.Code == http.StatusInternalServerError {
		t.Error("an unwired handler produced a 500; the nil guard did not hold")
	}
}

// Acceptance criterion 5: health reports accurately in both directions.
func TestHealthReflectsDependencyState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		probeErr   error
		wantStatus int
		wantBody   health.Status
	}{
		{"dependency up", nil, http.StatusOK, health.StatusHealthy},
		{"dependency down", errors.New("connection refused"), http.StatusServiceUnavailable, health.StatusUnhealthy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			testRouter(t, tc.probeErr).
				ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			var report health.Report
			if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if report.Status != tc.wantBody {
				t.Errorf("report status = %q, want %q", report.Status, tc.wantBody)
			}
			if rec.Header().Get(observability.HeaderRequestID) == "" {
				t.Error("every response must carry a request ID")
			}
		})
	}
}

func TestLivenessStaysUpWhileDependenciesAreDown(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter(t, errors.New("down")).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("liveness = %d, want 200: restarting the API cannot fix a database outage", rec.Code)
	}
}

func TestReadinessFollowsDependencies(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter(t, errors.New("down")).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("readiness = %d, want 503", rec.Code)
	}
}

func TestUnroutedPathsUseThePlatformErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter(t, nil).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Code != httpx.CodeNotFound {
		t.Errorf("code = %q, want not_found", body.Code)
	}
	if body.RequestID == "" {
		t.Error("error envelope must carry the request ID a client can quote")
	}
}

// Authenticated routes must refuse an unauthenticated caller before they reach
// a handler — which is what lets the test above wire a nil service safely.
func TestAuthenticatedRoutesRejectAnonymousCallers(t *testing.T) {
	router := testRouter(t, nil)

	for _, path := range []string{"/api/v1/me", "/api/v1/me/sessions"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", path, rec.Code)
		}
		var body httpx.ErrorBody
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if body.Code != httpx.CodeUnauthorized {
			t.Errorf("%s: code %q, want unauthorized", path, body.Code)
		}
	}
}

func TestBearerTokenIsRequiredInTheDocumentedForm(t *testing.T) {
	router := testRouter(t, nil)

	for _, header := range []string{"", "token-without-scheme", "Basic abc", "Bearer", "Bearer "} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q: status %d, want 401", header, rec.Code)
		}
	}
}
