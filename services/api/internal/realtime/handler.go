package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Handler is the transport the hub never had.
//
// Everything beneath it was built and unreachable: the hub, the channel
// grammar, the authorizer, the bounded buffers, the location coalescing — a
// complete gateway with no way in and nothing publishing into it. Document 018
// makes live job and driver state part of the product, and until now the only
// way to learn that a driver had moved was to ask again.
//
// It speaks Server-Sent Events rather than WebSocket. Document 047 names a
// WebSocket gateway and that remains the destination, but the whole of what
// this platform pushes today is one-directional — the hub has no inbound
// message path at all, only Events() — and SSE delivers that over plain HTTP
// with no new dependency, no upgrade handshake to get wrong, and automatic
// client reconnection. The hub is transport-agnostic by construction, so
// adding WebSocket later is a second handler beside this one rather than a
// change to anything underneath.
type Handler struct {
	hub     *Hub
	drivers DriverLookup
	// heartbeat keeps intermediaries from closing an idle stream.
	heartbeat time.Duration
}

// DriverLookup resolves the driver behind an account, so a driver may watch
// their own channel.
type DriverLookup interface {
	DriverIDForUser(ctx context.Context, userID string) (string, error)
}

// HeartbeatInterval is how often a comment line is written into an idle
// stream.
//
// Proxies and load balancers close connections that say nothing, and a ride
// with a stationary driver can legitimately be silent for minutes. Fifteen
// seconds is comfortably inside the common sixty-second idle timeouts.
const HeartbeatInterval = 15 * time.Second

// MaxChannelsPerConnection bounds one subscription request.
//
// Document 047 forbids arbitrary channel subscription. The grammar and the
// authorizer stop a client reaching channels it should not; this stops it
// asking for ten thousand it may.
const MaxChannelsPerConnection = 20

func NewHandler(hub *Hub, drivers DriverLookup) *Handler {
	return &Handler{hub: hub, drivers: drivers, heartbeat: HeartbeatInterval}
}

func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	mux.Handle("GET "+p+"/realtime", authenticate(http.HandlerFunc(h.stream)))
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}

	// The response must be streamed, not buffered. A transport that cannot
	// flush would deliver every event at once when the request ended, which
	// is the opposite of what this endpoint is for.
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, r, httpx.Internal("this server cannot stream"))
		return
	}

	requested := r.URL.Query()["channel"]
	if len(requested) == 0 {
		httpx.WriteError(w, r, httpx.Validation("a subscription needs at least one channel",
			map[string]string{"channel": "required"}))
		return
	}
	if len(requested) > MaxChannelsPerConnection {
		httpx.WriteError(w, r, httpx.Validation("too many channels",
			map[string]string{"channel": fmt.Sprintf("at most %d", MaxChannelsPerConnection)}))
		return
	}

	channels := make([]Channel, 0, len(requested))
	for _, raw := range requested {
		channel, err := ParseChannel(raw)
		if err != nil {
			httpx.WriteError(w, r, httpx.Validation("unknown channel",
				map[string]string{"channel": raw}))
			return
		}
		channels = append(channels, channel)
	}

	conn, err := h.hub.Connect(connectionID(principal), h.subscriberFor(r.Context(), principal))
	if err != nil {
		if errors.Is(err, ErrTooManyClients) {
			httpx.WriteError(w, r, httpx.RateLimited("too many open connections for this account"))
			return
		}
		httpx.WriteError(w, r, httpx.Internal("could not open the stream").WithCause(err))
		return
	}
	defer h.hub.Disconnect(conn)

	// Every channel or none. A partial success would leave the client
	// believing it is watching something it is not, and silence is
	// indistinguishable from nothing happening.
	for _, channel := range channels {
		if err := h.hub.Subscribe(conn, channel); err != nil {
			if errors.Is(err, ErrNotAuthorized) {
				httpx.WriteError(w, r, httpx.Forbidden("not permitted to watch "+channel.String()))
				return
			}
			httpx.WriteError(w, r, httpx.Internal("could not subscribe").WithCause(err))
			return
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Nginx buffers proxied responses by default, which would hold every event
	// until the stream ended.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// An immediate comment line so the client's connection callback fires
	// before the first real event, however long that takes to arrive.
	fmt.Fprint(w, ": subscribed\n\n")
	flusher.Flush()

	ticker := time.NewTicker(h.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-conn.Events():
			if !open {
				return
			}
			if err := writeEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
			// The client has just been written to, so the buffer has room.
			// Releasing the coalesced positions here is what turns "the
			// newest location replaces the pending one" into a position that
			// actually arrives.
			conn.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// subscriberFor builds what the authorizer checks against.
//
// A failed driver lookup is not fatal: it means this account is not a driver,
// which is the ordinary case for a customer. The empty DriverID then denies
// the driver channel, which is the correct answer.
func (h *Handler) subscriberFor(ctx context.Context, principal identity.Principal) Subscriber {
	sub := Subscriber{UserID: principal.UserID}
	for _, role := range principal.Roles {
		sub.Roles = append(sub.Roles, string(role))
	}
	if h.drivers != nil {
		if driverID, err := h.drivers.DriverIDForUser(ctx, principal.UserID); err == nil {
			sub.DriverID = driverID
		}
	}
	return sub
}

// connectionID is unique per connection, not per session: one account may hold
// several streams (a phone and a browser), and the hub bounds that count
// rather than collapsing them onto one identifier.
func connectionID(principal identity.Principal) string {
	return principal.UserID + ":" + principal.SessionID + ":" +
		time.Now().UTC().Format("20060102150405.000000000")
}

// writeEvent renders one envelope in the SSE wire format.
//
// The event id is written so a reconnecting client sends Last-Event-ID. This
// gateway does not replay — document 047 is explicit that it is not the system
// of record, and a client that missed something recovers by fetching state —
// but the header is standard, harmless, and useful in a log when someone is
// working out what a client last saw.
func writeEvent(w http.ResponseWriter, event Envelope) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if event.EventID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", event.EventID); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload)
	return err
}
