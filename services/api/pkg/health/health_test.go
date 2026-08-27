package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func ok(context.Context) error   { return nil }
func fail(context.Context) error { return errors.New("connection refused") }

func fixedClock() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func TestAllHealthy(t *testing.T) {
	checker := NewChecker("api", "test", []Check{
		{Name: "postgres", Critical: true, Probe: ok},
		{Name: "redis", Critical: true, Probe: ok},
		{Name: "nats", Critical: true, Probe: ok},
	}, WithClock(fixedClock))

	report := checker.Run(context.Background())
	if report.Status != StatusHealthy {
		t.Fatalf("status = %q, want healthy", report.Status)
	}
	if len(report.Dependencies) != 3 {
		t.Fatalf("got %d dependencies, want 3", len(report.Dependencies))
	}
}

// Acceptance criterion 5: the endpoint reports unhealthy when a dependency is
// stopped. A health check that cannot fail is decoration.
func TestCriticalDependencyFailureIsUnhealthy(t *testing.T) {
	checker := NewChecker("api", "test", []Check{
		{Name: "postgres", Critical: true, Probe: fail},
		{Name: "redis", Critical: true, Probe: ok},
	}, WithClock(fixedClock))

	report := checker.Run(context.Background())
	if report.Status != StatusUnhealthy {
		t.Fatalf("status = %q, want unhealthy", report.Status)
	}
	if report.Healthy() {
		t.Error("Healthy() must be false when a critical dependency is down")
	}

	var postgres DependencyResult
	for _, dep := range report.Dependencies {
		if dep.Name == "postgres" {
			postgres = dep
		}
	}
	if postgres.Status != StatusUnhealthy || postgres.Error == "" {
		t.Errorf("failing dependency must name its error, got %+v", postgres)
	}
}

func TestNonCriticalFailureOnlyDegrades(t *testing.T) {
	checker := NewChecker("api", "test", []Check{
		{Name: "postgres", Critical: true, Probe: ok},
		{Name: "object-storage", Critical: false, Probe: fail},
	}, WithClock(fixedClock))

	report := checker.Run(context.Background())
	if report.Status != StatusDegraded {
		t.Fatalf("status = %q, want degraded", report.Status)
	}
	if !report.Healthy() {
		t.Error("a degraded service should still take traffic")
	}
}

func TestProbeTimeoutDoesNotHangTheEndpoint(t *testing.T) {
	checker := NewChecker("api", "test", []Check{
		{Name: "slow", Critical: true, Probe: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}},
	}, WithClock(fixedClock), WithTimeout(20*time.Millisecond))

	done := make(chan Report, 1)
	go func() { done <- checker.Run(context.Background()) }()

	select {
	case report := <-done:
		if report.Status != StatusUnhealthy {
			t.Errorf("a timed-out probe must be unhealthy, got %q", report.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("health check did not respect its timeout")
	}
}

func TestHandlerAnswers503WhenUnhealthy(t *testing.T) {
	checker := NewChecker("api", "test", []Check{
		{Name: "postgres", Critical: true, Probe: fail},
	}, WithClock(fixedClock))

	rec := httptest.NewRecorder()
	Handler(checker).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}

	var report Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if report.Status != StatusUnhealthy {
		t.Errorf("body status = %q", report.Status)
	}
}

func TestLivenessIgnoresDependencies(t *testing.T) {
	rec := httptest.NewRecorder()
	LivenessHandler("api", "test").
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("liveness must not fail on a dependency outage, got %d", rec.Code)
	}
}
