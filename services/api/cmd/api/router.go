package main

import (
	"log/slog"
	"net/http"

	"github.com/sarmadkung/rideme/services/api/pkg/health"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/observability"
)

// newRouter wires the HTTP surface.
//
// Phase 1 serves health only. `/api/v1` (document 14) is registered as the
// versioned prefix so the first real endpoint has somewhere to land, but no
// route under it exists yet.
func newRouter(checker *health.Checker, service, version string, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /health", health.Handler(checker))
	mux.Handle("GET /health/live", health.LivenessHandler(service, version))
	mux.Handle("GET /health/ready", health.ReadinessHandler(checker))

	// Anything unrouted answers in the platform's error envelope.
	mux.Handle("/", httpx.NotFoundHandler())

	return observability.Chain(mux,
		observability.RequestContext(logger),
		observability.Recover(httpx.PanicHandler),
		observability.AccessLog(),
	)
}
