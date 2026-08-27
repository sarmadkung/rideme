// Package health aggregates dependency probes into the response served by the
// health endpoints (document 25).
//
// The point of a health check is that it can fail. A probe that always reports
// healthy tells an operator nothing, so each dependency is genuinely contacted
// and a failure is reported as a failure.
package health

import (
	"context"
	"sync"
	"time"
)

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

// Check is one dependency probe.
type Check struct {
	Name string
	// Critical dependencies make the whole service unhealthy; a non-critical
	// failure degrades it. Postgres is critical. Object storage is not — the
	// API can serve most traffic without it.
	Critical bool
	Probe    func(context.Context) error
}

type DependencyResult struct {
	Name      string `json:"name"`
	Status    Status `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

type Report struct {
	Status       Status             `json:"status"`
	Service      string             `json:"service"`
	Version      string             `json:"version"`
	CheckedAt    time.Time          `json:"checked_at"`
	Dependencies []DependencyResult `json:"dependencies"`
}

type Checker struct {
	service string
	version string
	timeout time.Duration
	checks  []Check
	now     func() time.Time
}

type Option func(*Checker)

// WithClock is used by tests; production uses time.Now.
func WithClock(now func() time.Time) Option {
	return func(c *Checker) { c.now = now }
}

func WithTimeout(d time.Duration) Option {
	return func(c *Checker) { c.timeout = d }
}

func NewChecker(service, version string, checks []Check, opts ...Option) *Checker {
	c := &Checker{
		service: service,
		version: version,
		timeout: 2 * time.Second,
		checks:  checks,
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Run probes every dependency concurrently and bounded by a timeout, so one
// hung dependency cannot hold the health endpoint open.
func (c *Checker) Run(ctx context.Context) Report {
	results := make([]DependencyResult, len(c.checks))

	var wg sync.WaitGroup
	for i, check := range c.checks {
		wg.Add(1)
		go func(i int, check Check) {
			defer wg.Done()
			results[i] = c.run(ctx, check)
		}(i, check)
	}
	wg.Wait()

	overall := StatusHealthy
	for i, result := range results {
		if result.Status == StatusHealthy {
			continue
		}
		if c.checks[i].Critical {
			overall = StatusUnhealthy
			break
		}
		overall = StatusDegraded
	}

	return Report{
		Status:       overall,
		Service:      c.service,
		Version:      c.version,
		CheckedAt:    c.now().UTC(),
		Dependencies: results,
	}
}

func (c *Checker) run(ctx context.Context, check Check) DependencyResult {
	probeCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	started := time.Now()
	err := check.Probe(probeCtx)
	latency := time.Since(started).Milliseconds()

	if err != nil {
		return DependencyResult{
			Name:      check.Name,
			Status:    StatusUnhealthy,
			LatencyMS: latency,
			Error:     err.Error(),
		}
	}
	return DependencyResult{Name: check.Name, Status: StatusHealthy, LatencyMS: latency}
}

// Healthy reports whether the service should receive traffic.
func (r Report) Healthy() bool { return r.Status != StatusUnhealthy }
