package payments

import (
	"log/slog"
	"net/http"

	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Handler tells a customer how they may pay.
type Handler struct {
	store   *Store
	gateway *Gateway
	logger  *slog.Logger
}

func NewHandler(store *Store, gateway *Gateway, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: store, gateway: gateway, logger: logger}
}

// Routes registers the method list.
//
// Authenticated, though it holds nothing personal: an unauthenticated list
// would be a public statement of which providers this platform uses, and that
// is reconnaissance for anybody probing the webhook route.
func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	mux.Handle("GET "+p+"/payment-methods", authenticate(http.HandlerFunc(h.list)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	configured, err := h.store.Configured(r.Context())
	if err != nil {
		httpx.WriteError(w, r, httpx.Internal("could not read payment methods").WithCause(err))
		return
	}

	offered, misconfigured := h.gateway.Available(configured)
	for _, method := range misconfigured {
		// Enabled in the database, with no provider in this binary. The
		// customer is correctly not shown it; somebody needs to know why.
		h.logger.Error("a payment method is enabled with no provider registered",
			slog.String("method", string(method)))
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"methods": offered})
}
