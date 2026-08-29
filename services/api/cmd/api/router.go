package main

import (
	"log/slog"
	"net/http"

	"github.com/sarmadkung/rideme/services/api/internal/booking"
	"github.com/sarmadkung/rideme/services/api/internal/driver"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/pkg/authn"
	"github.com/sarmadkung/rideme/services/api/pkg/health"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/observability"
)

// newRouter wires the HTTP surface.
//
// Health sits outside the versioned prefix deliberately: an operator probing
// liveness should not have to know the API version. Everything else lives
// under `/api/v1` (document 14).
func newRouter(
	checker *health.Checker,
	identityHandler *identity.Handler,
	bookingHandler *booking.Handler,
	driverHandler *driver.Handler,
	issuer *authn.Issuer,
	service, version string,
	logger *slog.Logger,
) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /health", health.Handler(checker))
	mux.Handle("GET /health/live", health.LivenessHandler(service, version))
	mux.Handle("GET /health/ready", health.ReadinessHandler(checker))

	authenticate := identity.Authenticate(issuer)
	identityHandler.Routes(mux, authenticate)
	if bookingHandler != nil {
		bookingHandler.Routes(mux, authenticate)
	}
	if driverHandler != nil {
		driverHandler.Routes(mux, authenticate)
	}

	// Anything unrouted answers in the platform's error envelope.
	mux.Handle("/", httpx.NotFoundHandler())

	return observability.Chain(mux,
		observability.RequestContext(logger),
		observability.Recover(httpx.PanicHandler),
		observability.AccessLog(),
	)
}
