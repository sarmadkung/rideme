package payments_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sarmadkung/rideme/services/api/internal/payments"
)

type recordedEvent struct {
	provider, eventID, eventType string
	signatureOK                  bool
}

type fakeEvents struct {
	events []recordedEvent
	seen   map[string]bool
	fail   bool
}

func newFakeEvents() *fakeEvents { return &fakeEvents{seen: map[string]bool{}} }

func (e *fakeEvents) RecordWebhook(_ context.Context, provider, eventID, eventType string,
	_ []byte, signatureOK bool, _ string) (bool, error) {
	if e.fail {
		return false, errors.New("database is down")
	}
	e.events = append(e.events, recordedEvent{provider, eventID, eventType, signatureOK})
	key := provider + ":" + eventID
	if e.seen[key] {
		return false, nil
	}
	e.seen[key] = true
	return true, nil
}

func webhookServer(t *testing.T, events *fakeEvents, gateway *payments.Gateway) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	payments.NewWebhookHandler(events, gateway, discard()).Routes(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func callback(t *testing.T, url, body, signature string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if signature != "" {
		req.Header.Set(payments.SignatureHeader, signature)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// The platform's actual state: no provider registered, so every callback is
// refused before its body is read. An open webhook endpoint that accepts
// anything is a way to tell the platform a payment succeeded when it did not.
func TestACallbackForAnUnregisteredProviderIsRefused(t *testing.T) {
	events := newFakeEvents()
	server := webhookServer(t, events, payments.NewGateway())

	resp := callback(t, server.URL+"/api/v1/webhooks/payments/somebank",
		`{"id":"evt_1","type":"payment.captured"}`, "sig")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
	if len(events.events) != 0 {
		t.Error("a callback from nobody was recorded")
	}
}

// Document 058 wants the evidence. A burst of failing signatures is somebody
// probing, and an endpoint that drops what it rejects cannot tell anyone.
func TestABadSignatureIsRecordedAndRefused(t *testing.T) {
	events := newFakeEvents()
	gateway := payments.NewGateway(fakeProvider{
		name: "somebank", methods: []payments.Method{payments.MethodCard},
		verify: errors.New("bad signature"),
	})
	server := webhookServer(t, events, gateway)

	resp := callback(t, server.URL+"/api/v1/webhooks/payments/somebank",
		`{"id":"evt_2","type":"payment.captured"}`, "wrong")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
	if len(events.events) != 1 {
		t.Fatalf("%d events recorded, want 1 — a rejected callback is still evidence", len(events.events))
	}
	if events.events[0].signatureOK {
		t.Error("a failed signature was recorded as verified")
	}
}

// Document 052: "Webhook processing must be idempotent." A provider replaying
// a capture must not capture twice, and 200 is what stops it retrying forever.
func TestAReplayedCallbackIsAcceptedOnce(t *testing.T) {
	events := newFakeEvents()
	gateway := payments.NewGateway(fakeProvider{
		name: "somebank", methods: []payments.Method{payments.MethodCard},
	})
	server := webhookServer(t, events, gateway)

	for i := 0; i < 3; i++ {
		resp := callback(t, server.URL+"/api/v1/webhooks/payments/somebank",
			`{"id":"evt_3","type":"payment.captured"}`, "ok")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("attempt %d: status %d, want 200", i+1, resp.StatusCode)
		}
		resp.Body.Close()
	}
	if len(events.seen) != 1 {
		t.Errorf("%d distinct events, want 1", len(events.seen))
	}
}

// A callback whose id cannot be read must still be stored rather than dropped,
// and a replay of the same body must still deduplicate.
func TestACallbackWithNoIdentifiableIdStillDeduplicates(t *testing.T) {
	events := newFakeEvents()
	gateway := payments.NewGateway(fakeProvider{
		name: "somebank", methods: []payments.Method{payments.MethodCard},
	})
	server := webhookServer(t, events, gateway)

	for i := 0; i < 2; i++ {
		callback(t, server.URL+"/api/v1/webhooks/payments/somebank",
			`{"something":"unexpected"}`, "ok").Body.Close()
	}

	if len(events.events) != 2 {
		t.Fatalf("%d recorded, want 2", len(events.events))
	}
	if events.events[0].eventID == "" {
		t.Error("an unidentifiable callback got an empty id")
	}
	if len(events.seen) != 1 {
		t.Error("the same body was treated as two different events")
	}
}

// Losing a callback silently is how a captured payment never reaches the
// ledger. A failure to record must ask the provider to try again.
func TestAFailureToRecordAsksTheProviderToRetry(t *testing.T) {
	events := newFakeEvents()
	events.fail = true
	gateway := payments.NewGateway(fakeProvider{
		name: "somebank", methods: []payments.Method{payments.MethodCard},
	})
	server := webhookServer(t, events, gateway)

	resp := callback(t, server.URL+"/api/v1/webhooks/payments/somebank",
		`{"id":"evt_4"}`, "ok")
	defer resp.Body.Close()

	if resp.StatusCode < 500 {
		t.Errorf("status %d, want a 5xx so the provider retries", resp.StatusCode)
	}
}
