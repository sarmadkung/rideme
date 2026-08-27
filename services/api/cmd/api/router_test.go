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
	return newRouter(checker, "api", "test", logger)
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
