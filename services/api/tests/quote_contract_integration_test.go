//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/booking"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// The quote endpoint used to write an ad-hoc map that did not match its own
// declared QuoteResponse: the struct said total_minor and currency, the
// handler sent total and lines. The generated TypeScript client followed the
// struct, so every call to it would have failed to parse a perfectly valid
// response.
//
// The contract generator cannot catch that. It compares Go structs to
// TypeScript and never sees what a handler actually writes. This test closes
// that gap for the endpoint it bit, by decoding a real response strictly
// against the declared type.

func TestTheQuoteEndpointSendsExactlyItsDeclaredShape(t *testing.T) {
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)

	handler := booking.NewHandler(h.service, h.jobs, providers.NewStore(h.pool))
	mux := http.NewServeMux()
	// The authenticator stands in for real middleware: this test is about the
	// response body, not about who may ask for it.
	handler.Routes(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(
				identity.ContextWithPrincipal(r.Context(), identity.Principal{UserID: userID})))
		})
	})

	body, err := json.Marshal(map[string]any{
		"job_type":     "RIDE",
		"vehicle_type": "CAR",
		"city":         city,
		"stops": []map[string]any{
			{"type": "PICKUP", "latitude": 31.5204, "longitude": 74.3587},
			{"type": "DROPOFF", "latitude": 31.5880, "longitude": 74.3150},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, httpx.APIVersionPrefix+"/quotes", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// DisallowUnknownFields is the whole point: a handler that sends a field
	// the declared type does not have fails here rather than in a client.
	decoder := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	decoder.DisallowUnknownFields()
	var quote booking.QuoteResponse
	if err := decoder.Decode(&quote); err != nil {
		t.Fatalf("the response does not match QuoteResponse: %v\nbody: %s", err, rec.Body.String())
	}

	// And every declared field is actually populated, so the shape matching is
	// not achieved by sending nothing.
	if quote.QuoteID == "" {
		t.Error("quote_id is empty")
	}
	if quote.Total.Minor <= 0 {
		t.Errorf("total = %d minor units", quote.Total.Minor)
	}
	if quote.Total.Currency == "" {
		t.Error("the total carries no currency")
	}
	if len(quote.Lines) == 0 {
		t.Error("the fare breakdown is empty; a customer cannot see what they are paying for")
	}
	if quote.DistanceMeters <= 0 || quote.DurationSeconds <= 0 {
		t.Errorf("route = %dm in %ds", quote.DistanceMeters, quote.DurationSeconds)
	}
	if quote.RouteConfidence == "" {
		t.Error("route_confidence is empty; document 096 forbids presenting an estimate as exact")
	}
	if quote.ExpiresAt.IsZero() {
		t.Error("expires_at is zero")
	}

	// The breakdown must add up to the total it is a breakdown of.
	var sum int64
	for _, line := range quote.Lines {
		sum += line.Amount.Minor
	}
	if sum != quote.Total.Minor {
		t.Fatalf("the lines sum to %d but the total says %d", sum, quote.Total.Minor)
	}
}
