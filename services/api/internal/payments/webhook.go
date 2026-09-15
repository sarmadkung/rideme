package payments

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// The webhook receiver (documents 052, 058, 059).
//
// `finance.RecordWebhook` and `finance.VerifySignature` were written, tested
// and never called. This is the caller. It does nothing today because no
// provider is registered — every callback is refused at the first line — and
// that is the correct behaviour rather than a placeholder: an open webhook
// endpoint that accepts anything is a way to tell the platform a payment
// succeeded when it did not.

// MaxWebhookBody bounds what a caller can post. Provider callbacks are small;
// an unbounded read on a public endpoint is an easy way to exhaust a process.
const MaxWebhookBody = 256 << 10

// Events records provider callbacks, deduplicated by event id.
type Events interface {
	RecordWebhook(ctx context.Context, provider, eventID, eventType string,
		payload []byte, signatureOK bool, intentID string) (isNew bool, err error)
}

// WebhookHandler receives provider callbacks.
type WebhookHandler struct {
	events  Events
	gateway *Gateway
	logger  *slog.Logger
}

func NewWebhookHandler(events Events, gateway *Gateway, logger *slog.Logger) *WebhookHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookHandler{events: events, gateway: gateway, logger: logger}
}

// Routes registers the callback path.
//
// Unauthenticated, necessarily — a provider has no session. The signature is
// the authentication, which is why nothing happens before it verifies.
func (h *WebhookHandler) Routes(mux *http.ServeMux) {
	const p = httpx.APIVersionPrefix
	mux.Handle("POST "+p+"/webhooks/payments/{provider}", http.HandlerFunc(h.receive))
}

// SignatureHeader is where providers conventionally put it. A provider that
// uses a different header is a reason to read it in that provider's own
// implementation, not to accept an unsigned callback.
const SignatureHeader = "X-Signature"

func (h *WebhookHandler) receive(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("provider")

	provider, ok := h.providerNamed(name)
	if !ok {
		// An unknown provider is refused before the body is read. This is the
		// state the platform is in today — no provider is registered — and it
		// is what makes this endpoint inert rather than dangerous.
		h.logger.Warn("a webhook arrived for an unregistered provider",
			slog.String("provider", name))
		httpx.WriteError(w, r, httpx.NotFound("unknown payment provider"))
		return
	}

	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxWebhookBody))
	if err != nil {
		httpx.WriteError(w, r, httpx.Validation("could not read the callback body", nil))
		return
	}

	signatureErr := provider.VerifyWebhook(payload, r.Header.Get(SignatureHeader))

	// Recorded either way, verified or not. Document 058 wants the evidence:
	// a burst of failing signatures is somebody probing, and an endpoint that
	// drops what it rejects cannot tell anyone that is happening.
	eventID, eventType := eventIdentity(payload)
	isNew, recordErr := h.events.RecordWebhook(r.Context(), name, eventID, eventType,
		payload, signatureErr == nil, "")
	if recordErr != nil {
		h.logger.Error("could not record a provider callback",
			slog.String("provider", name), slog.String("error", recordErr.Error()))
		// 500 so the provider retries. Losing a callback silently is how a
		// captured payment never reaches the ledger.
		httpx.WriteError(w, r, httpx.Internal("could not record the callback"))
		return
	}

	if signatureErr != nil {
		h.logger.Error("a provider callback failed signature verification",
			slog.String("provider", name), slog.String("event_id", eventID))
		httpx.WriteError(w, r, httpx.Unauthorized("invalid signature"))
		return
	}

	if !isNew {
		// Document 052: "Webhook processing must be idempotent." A provider
		// replaying a capture must not capture twice, and 200 is what stops it
		// retrying forever.
		w.WriteHeader(http.StatusOK)
		return
	}

	// Acting on the event — capturing an intent, posting the movement — is the
	// next slice, and it needs a provider to exist before it can be written
	// against anything real. The callback is durably recorded and unprocessed,
	// which is the state `payment_webhook_events.processed_at IS NULL` exists
	// to represent.
	h.logger.Info("provider callback recorded and awaiting processing",
		slog.String("provider", name), slog.String("event_id", eventID),
		slog.String("event_type", eventType))
	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) providerNamed(name string) (Provider, bool) {
	if h.gateway == nil || name == "" {
		return nil, false
	}
	for _, method := range []Method{MethodCard, MethodWallet, MethodBank} {
		if provider, ok := h.gateway.ProviderFor(method); ok && provider.Name() == name {
			return provider, true
		}
	}
	return nil, false
}

// eventIdentity pulls the provider's own event id out of the payload.
//
// Deliberately tolerant: providers disagree about the field name, and a
// callback whose id cannot be read must still be stored rather than dropped.
// An unreadable id falls back to a hash of the payload, so a replay of the
// same body still deduplicates.
func eventIdentity(payload []byte) (eventID, eventType string) {
	var envelope struct {
		ID    string `json:"id"`
		Event string `json:"event_id"`
		Type  string `json:"type"`
		Kind  string `json:"event_type"`
	}
	if err := decodeJSON(payload, &envelope); err == nil {
		eventID = firstNonEmpty(envelope.ID, envelope.Event)
		eventType = firstNonEmpty(envelope.Type, envelope.Kind)
	}
	if eventID == "" {
		eventID = hashOf(payload)
	}
	if eventType == "" {
		eventType = "unknown"
	}
	return eventID, eventType
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var errNotJSON = errors.New("payments: the callback body is not an object")
