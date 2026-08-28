package events_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/events"
)

func valid() events.Envelope {
	return events.Envelope{
		EventID:   "01J000000000000000000000",
		EventName: "ride.booked",
		ActorID:   "user_1",
		Timestamp: time.Date(2026, 8, 28, 10, 30, 0, 0, time.UTC),
		Source:    "api",
	}
}

func TestValidateAcceptsADocumentedEnvelope(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAcceptsTheDocumentedEventNames(t *testing.T) {
	// The names document 150 gives as examples must all be accepted.
	for _, name := range []events.Name{
		"ride.requested", "ride.booked", "ride.cancelled",
		"delivery.created", "delivery.assigned", "delivery.completed",
		"payment.succeeded",
	} {
		envelope := valid()
		envelope.EventName = name
		if err := envelope.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestValidateRejectsMalformedNames(t *testing.T) {
	for _, name := range []events.Name{
		"ride", "Ride.Booked", "ride..booked", ".booked", "ride.", "ride booked", "ride-booked",
	} {
		envelope := valid()
		envelope.EventName = name
		if err := envelope.Validate(); !errors.Is(err, events.ErrNameMalformed) {
			t.Fatalf("%q: want ErrNameMalformed, got %v", name, err)
		}
	}
}

func TestValidateRequiresIdentity(t *testing.T) {
	envelope := valid()
	envelope.ActorID = ""
	if err := envelope.Validate(); !errors.Is(err, events.ErrNoActor) {
		t.Fatalf("want ErrNoActor, got %v", err)
	}

	// anonymous_id is the documented alternative "where applicable".
	envelope.AnonymousID = "anon_1"
	if err := envelope.Validate(); err != nil {
		t.Fatalf("anonymous_id should satisfy identity: %v", err)
	}
}

func TestValidateRejectsNonUTCTimestamps(t *testing.T) {
	envelope := valid()
	envelope.Timestamp = time.Date(2026, 8, 28, 10, 30, 0, 0, time.FixedZone("PKT", 5*60*60))
	if err := envelope.Validate(); !errors.Is(err, events.ErrTimestampNotUTC) {
		t.Fatalf("want ErrTimestampNotUTC, got %v", err)
	}

	envelope.Timestamp = time.Time{}
	if err := envelope.Validate(); !errors.Is(err, events.ErrTimestampUnset) {
		t.Fatalf("want ErrTimestampUnset, got %v", err)
	}
}

func TestValidateRequiresIDAndSource(t *testing.T) {
	envelope := valid()
	envelope.EventID = ""
	if err := envelope.Validate(); !errors.Is(err, events.ErrIDRequired) {
		t.Fatalf("want ErrIDRequired, got %v", err)
	}

	envelope = valid()
	envelope.Source = ""
	if err := envelope.Validate(); !errors.Is(err, events.ErrSourceRequired) {
		t.Fatalf("want ErrSourceRequired, got %v", err)
	}
}

func TestSerializationRoundTripsAndNormalisesToUTC(t *testing.T) {
	envelope := valid()
	envelope.SessionID = "session_1"
	envelope.CorrelationID = "corr_1"
	envelope.Properties = map[string]any{"vehicle_type": "motorcycle"}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	var decoded events.Envelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Timestamp.Equal(envelope.Timestamp) {
		t.Fatalf("timestamp changed: %v vs %v", decoded.Timestamp, envelope.Timestamp)
	}
	if decoded.EventName != envelope.EventName || decoded.CorrelationID != "corr_1" {
		t.Fatalf("round trip lost fields: %+v", decoded)
	}
}

func TestOptionalFieldsAreOmittedFromTheWire(t *testing.T) {
	encoded, err := json.Marshal(valid())
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"event_id":"01J000000000000000000000","event_name":"ride.booked","actor_id":"user_1","timestamp":"2026-08-28T10:30:00Z","source":"api"}`
	if string(encoded) != want {
		t.Fatalf("want %s\ngot  %s", want, encoded)
	}
}

func TestInvalidEnvelopesCannotBeEncodedOrDecoded(t *testing.T) {
	envelope := valid()
	envelope.EventName = "nonsense"
	if _, err := json.Marshal(envelope); err == nil {
		t.Fatal("encoded an invalid envelope")
	}

	var decoded events.Envelope
	if err := json.Unmarshal([]byte(`{"event_id":"1","event_name":"nonsense","actor_id":"u","timestamp":"2026-08-28T10:30:00Z","source":"api"}`), &decoded); err == nil {
		t.Fatal("decoded an invalid envelope")
	}
}
