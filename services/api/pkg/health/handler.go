package health

import (
	"net/http"

	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Handler serves the aggregated report. An unhealthy service answers 503 so a
// load balancer or orchestrator can act on it without parsing the body.
func Handler(checker *Checker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		report := checker.Run(r.Context())

		status := http.StatusOK
		if !report.Healthy() {
			status = http.StatusServiceUnavailable
		}
		httpx.WriteJSON(w, r, status, report)
	})
}

// LivenessHandler answers whether the process is running. It deliberately
// touches no dependency: restarting the API does not fix a broken database, and
// a liveness probe that fails on a dependency outage turns one outage into a
// restart loop.
func LivenessHandler(service, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, r, http.StatusOK, map[string]string{
			"status":  string(StatusHealthy),
			"service": service,
			"version": version,
		})
	})
}

// ReadinessHandler answers whether the service can serve traffic, which does
// depend on its critical dependencies.
func ReadinessHandler(checker *Checker) http.Handler {
	return Handler(checker)
}
