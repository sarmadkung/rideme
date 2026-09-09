package main

import (
	"log/slog"
	"net/http"

	"github.com/sarmadkung/rideme/services/api/internal/booking"
	"github.com/sarmadkung/rideme/services/api/internal/dispatch"
	"github.com/sarmadkung/rideme/services/api/internal/driver"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/internal/places"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
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
	placesHandler *places.Handler,
	merchantHandler *merchant.Handler,
	groceryHandler *merchant.CustomerHandler,
	offerHandler *dispatch.Handler,
	trackHandler *tracking.Handler,
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
	// Absent when no geocoder is configured, so the routes 404 rather than
	// existing and always failing.
	if placesHandler != nil {
		placesHandler.Routes(mux, authenticate)
	}
	if merchantHandler != nil {
		merchantHandler.Routes(mux, authenticate)
	}
	if groceryHandler != nil {
		groceryHandler.Routes(mux, authenticate)
	}
	// The offer responses, and the trip the accepted offer becomes.
	if offerHandler != nil {
		offerHandler.Routes(mux, authenticate)
	}
	if trackHandler != nil {
		trackHandler.Routes(mux, authenticate)
	}

	// Anything unrouted answers in the platform's error envelope.
	mux.Handle("/", httpx.NotFoundHandler())

	return observability.Chain(mux,
		observability.RequestContext(logger),
		observability.Recover(httpx.PanicHandler),
		observability.AccessLog(),
	)
}
