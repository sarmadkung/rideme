// Package events defines the analytics event envelope from document 150.
//
// This is CAP-5's Phase 2 increment and it is deliberately the whole of it:
// an envelope and its validation rules, and nothing else. There is no
// collection, no stream, no storage and no consumer — those follow their data
// in Phase 14, and building them now would mean building them against no
// events.
//
// The envelope is early for one reason: retrofitting it later means revisiting
// every emission site in the platform. Agreeing the shape before the first
// producer exists costs almost nothing.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Name is an event name in the documented `domain.action` form — `ride.booked`,
// `payment.succeeded` (document 150).
//
// Document 150 also states that event names are versioned but specifies no
// versioning scheme. None is invented here: the shape below is validated, and
// the scheme is recorded as open in docs/IMPLEMENTATION_STATUS.md. It blocks
// nothing, because no producer exists yet.
type Name string

// NamePattern permits `domain.action` and deeper dotted segments, lowercase
// with underscores inside a segment.
//
// It is exported because the generated Zod schema validates client-side event
// names against this same expression. One pattern, two runtimes — the same
// reasoning as ADR-007 applied to a rule rather than a shape.
const NamePattern = `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`

var namePattern = regexp.MustCompile(NamePattern)

var (
	ErrNameEmpty       = errors.New("events: event_name is required")
	ErrNameMalformed   = errors.New("events: event_name must be dotted lowercase, as in ride.booked")
	ErrIDRequired      = errors.New("events: event_id is required")
	ErrSourceRequired  = errors.New("events: source is required")
	ErrTimestampUnset  = errors.New("events: timestamp is required")
	ErrTimestampNotUTC = errors.New("events: timestamp must be UTC")
	ErrNoActor         = errors.New("events: one of actor_id or anonymous_id is required")
)

// Envelope carries the fields document 150 lists, and only those.
//
// Properties is deliberately untyped: no event has a documented property
// schema yet, and inventing one would be inventing specification. A monetary
// value placed in Properties must be a money.Amount, never a bare number —
// BD-07 admits no exceptions, including here.
type Envelope struct {
	EventID       string         `json:"event_id"`
	EventName     Name           `json:"event_name"`
	ActorID       string         `json:"actor_id,omitempty"`
	AnonymousID   string         `json:"anonymous_id,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
	Source        string         `json:"source"`
	SessionID     string         `json:"session_id,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Properties    map[string]any `json:"properties,omitempty"`
}

// Validate enforces the rules document 150 states: a well-formed name, a UTC
// timestamp, and enough identity to attribute the event.
//
// "Sensitive information is excluded" is also a documented rule, but it is a
// judgement about content that no validator can make. It is enforced by review
// at each emission site, not here, and saying so is more honest than a check
// that would pass everything.
func (e Envelope) Validate() error {
	if e.EventID == "" {
		return ErrIDRequired
	}
	if e.EventName == "" {
		return ErrNameEmpty
	}
	if !namePattern.MatchString(string(e.EventName)) {
		return fmt.Errorf("%w: %q", ErrNameMalformed, e.EventName)
	}
	if e.Source == "" {
		return ErrSourceRequired
	}
	if e.Timestamp.IsZero() {
		return ErrTimestampUnset
	}
	// An event whose timestamp carries a local offset cannot be ordered
	// against one that does not. Document 150 requires UTC; this is where that
	// stops being a convention and becomes a rule.
	if _, offset := e.Timestamp.Zone(); offset != 0 {
		return fmt.Errorf("%w: got offset %ds", ErrTimestampNotUTC, offset)
	}
	if e.ActorID == "" && e.AnonymousID == "" {
		return ErrNoActor
	}
	return nil
}

// MarshalJSON emits a validated envelope with its timestamp in RFC 3339 UTC.
// An invalid envelope fails here rather than reaching a consumer.
func (e Envelope) MarshalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	type wire Envelope
	out := wire(e)
	out.Timestamp = e.Timestamp.UTC()
	return json.Marshal(out)
}

// UnmarshalJSON decodes and validates, so an invalid envelope cannot enter the
// system by being decoded.
func (e *Envelope) UnmarshalJSON(data []byte) error {
	type wire Envelope
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	envelope := Envelope(decoded)
	envelope.Timestamp = envelope.Timestamp.UTC()
	if err := envelope.Validate(); err != nil {
		return err
	}
	*e = envelope
	return nil
}
