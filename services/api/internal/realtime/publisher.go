package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Publisher is what the rest of the platform pushes through.
//
// The hub had no writers at all: every event type document 018 lists was
// declared as a constant and never constructed. This is the half that makes
// them real, and it is a separate type from the Hub so the packages that
// announce things depend on a method set of two rather than on a gateway.
//
// Nothing here returns an error. A notification that fails to deliver must
// never fail the operation that caused it — a driver's position reached the
// database, a job's status changed, and whether a websocket saw it is a
// different question. Document 047 is explicit that this path is not the
// system of record, so a client that missed an event recovers by asking.
type Publisher struct {
	hub *Hub
	now func() time.Time
}

func NewPublisher(hub *Hub, now func() time.Time) *Publisher {
	if now == nil {
		now = time.Now
	}
	return &Publisher{hub: hub, now: now}
}

// JobStatus is the payload behind job.status_changed.
type JobStatus struct {
	JobID    string `json:"job_id"`
	Status   string `json:"status"`
	JobType  string `json:"job_type,omitempty"`
	DriverID string `json:"driver_id,omitempty"`
}

// JobChanged announces a job's new status.
//
// Published to the job's own channel and to the customer's, because a customer
// watching their booking has not necessarily subscribed to the job: they know
// their own user id before they know a job exists. The driver's channel gets
// it too when one is assigned, so a driver's phone learns that a trip was
// cancelled out from under it without polling.
func (p *Publisher) JobChanged(_ context.Context, jobID, requesterUserID, driverID, jobType, status string) {
	if p == nil || jobID == "" {
		return
	}
	event := p.envelope(EventJobStatusChanged, jobID, JobStatus{
		JobID: jobID, Status: status, JobType: jobType, DriverID: driverID,
	})
	p.hub.Publish(Channel{Kind: ChannelJob, ID: jobID}, event)
	if requesterUserID != "" {
		p.hub.Publish(Channel{Kind: ChannelUser, ID: requesterUserID}, event)
	}
	if driverID != "" {
		p.hub.Publish(Channel{Kind: ChannelDriver, ID: driverID}, event)
	}
}

// DriverPosition is the payload behind driver.location.
type DriverPosition struct {
	DriverID   string    `json:"driver_id"`
	JobID      string    `json:"job_id,omitempty"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	HeadingDeg *float64  `json:"heading_deg,omitempty"`
	SpeedMPS   *float64  `json:"speed_mps,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
}

// DriverMoved announces a new position.
//
// The job channel is the one that matters to a customer: document 102 scopes
// location to the active service, and the authorizer will not let a customer
// subscribe to a driver's own channel. Publishing to both is what lets a
// driver's app watch itself and a customer watch only the trip they are on.
//
// This is the high-frequency event the connection buffers coalesce, so a
// client that falls behind receives where the driver is rather than a queue of
// where they were.
func (p *Publisher) DriverMoved(_ context.Context, position DriverPosition) {
	if p == nil || position.DriverID == "" {
		return
	}
	event := p.envelope(EventDriverLocation, position.DriverID, position)
	p.hub.Publish(Channel{Kind: ChannelDriver, ID: position.DriverID}, event)
	if position.JobID != "" {
		// The resource is the job here, so the coalescing key is the trip
		// being watched rather than the driver — a customer watching one job
		// must not have its position replaced by the same driver's next one.
		jobEvent := event
		jobEvent.ResourceID = position.JobID
		p.hub.Publish(Channel{Kind: ChannelJob, ID: position.JobID}, jobEvent)
	}
}

func (p *Publisher) envelope(kind EventType, resourceID string, payload any) Envelope {
	return Envelope{
		EventID:    newEventID(),
		Type:       kind,
		Version:    1,
		OccurredAt: p.now().UTC(),
		ResourceID: resourceID,
		Payload:    payload,
	}
}

// newEventID is random rather than sequential. An id a client can guess or
// count tells it how many events the platform emitted, and a gap in a sequence
// invites a client to believe it missed something it can ask to have replayed
// — which this gateway deliberately cannot do.
func newEventID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Nothing here is a security boundary and an event with no id is
		// still deliverable, so a failed read degrades rather than panics.
		return ""
	}
	return hex.EncodeToString(buf[:])
}
