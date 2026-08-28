// Package jobs is the platform's universal work abstraction (document 04).
//
// A ride, a parcel delivery, a grocery order and a cargo haul are all a Job
// with a different Type. This is the single most consequential modelling
// decision in the codebase: dispatch, tracking, pricing, payment and the
// operator console each learn one shape instead of four, and a fifth service
// costs a type constant rather than a subsystem.
//
// The service lifecycles specialize this core. They never fork it.
package jobs

import (
	"errors"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
	"github.com/sarmadkung/rideme/services/api/pkg/statemachine"
)

// Type is the kind of work. Document 04 fixes the five.
type Type string

const (
	TypeRide    Type = "RIDE"
	TypeParcel  Type = "PARCEL"
	TypeGrocery Type = "GROCERY"
	TypeCargo   Type = "CARGO"
	TypeFreight Type = "FREIGHT"
)

var AllTypes = []Type{TypeRide, TypeParcel, TypeGrocery, TypeCargo, TypeFreight}

func (t Type) Valid() bool {
	for _, known := range AllTypes {
		if t == known {
			return true
		}
	}
	return false
}

// Status is a job's position in the lifecycle document 15 defines.
type Status string

const (
	StatusDraft      Status = "DRAFT"
	StatusQuoted     Status = "QUOTED"
	StatusRequested  Status = "REQUESTED"
	StatusSearching  Status = "SEARCHING"
	StatusAssigned   Status = "ASSIGNED"
	StatusAccepted   Status = "ACCEPTED"
	StatusArriving   Status = "ARRIVING"
	StatusAtPickup   Status = "AT_PICKUP"
	StatusInProgress Status = "IN_PROGRESS"
	StatusAtDropoff  Status = "AT_DROPOFF"
	StatusCompleted  Status = "COMPLETED"

	StatusCancelled Status = "CANCELLED"
	StatusFailed    Status = "FAILED"
	StatusExpired   Status = "EXPIRED"
	StatusDisputed  Status = "DISPUTED"
)

// Machine is the job lifecycle exactly as document 15 draws it:
//
//	DRAFT -> QUOTED -> REQUESTED -> SEARCHING -> ASSIGNED -> ACCEPTED
//	      -> ARRIVING -> AT_PICKUP -> IN_PROGRESS -> AT_DROPOFF -> COMPLETED
//
// with CANCELLED, FAILED, EXPIRED and DISPUTED terminal.
//
// Cancellation is reachable from every live state up to AT_DROPOFF, because in
// practice a job can be called off at any point before it finishes; what
// cancelling *costs* is a separate question and an unresolved one (BD-01).
// After COMPLETED the only exit is DISPUTED — a finished job is not
// un-finished, it is disputed.
var Machine = statemachine.New(statemachine.Definition[Status]{
	Name:    "job",
	Initial: StatusDraft,
	Transitions: map[Status][]Status{
		StatusDraft:     {StatusQuoted, StatusRequested, StatusCancelled, StatusExpired},
		StatusQuoted:    {StatusRequested, StatusCancelled, StatusExpired},
		StatusRequested: {StatusSearching, StatusCancelled, StatusExpired, StatusFailed},
		// EXPIRED is the terminal path out of a search that finds nobody
		// (BD-04 — the durations are unresolved, the shape is not).
		StatusSearching:  {StatusAssigned, StatusCancelled, StatusExpired, StatusFailed},
		StatusAssigned:   {StatusAccepted, StatusSearching, StatusCancelled, StatusExpired, StatusFailed},
		StatusAccepted:   {StatusArriving, StatusSearching, StatusCancelled, StatusFailed},
		StatusArriving:   {StatusAtPickup, StatusCancelled, StatusFailed},
		StatusAtPickup:   {StatusInProgress, StatusCancelled, StatusFailed},
		StatusInProgress: {StatusAtDropoff, StatusFailed},
		StatusAtDropoff:  {StatusCompleted, StatusFailed},
		StatusCompleted:  {StatusDisputed},
	},
	Terminal: []Status{StatusCancelled, StatusFailed, StatusExpired, StatusDisputed},
})

// StopType distinguishes the ends of a job from the points between them.
type StopType string

const (
	StopPickup   StopType = "PICKUP"
	StopDropoff  StopType = "DROPOFF"
	StopWaypoint StopType = "WAYPOINT"
)

// Coordinate is a WGS-84 point.
type Coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (c Coordinate) Valid() bool {
	return c.Latitude >= -90 && c.Latitude <= 90 &&
		c.Longitude >= -180 && c.Longitude <= 180 &&
		!(c.Latitude == 0 && c.Longitude == 0) // null island is a bug, not a location
}

// Stop is one place a job visits.
type Stop struct {
	ID           string
	JobID        string
	Sequence     int
	Type         StopType
	Location     Coordinate
	Address      string
	ContactName  string
	ContactPhone string
	ArrivedAt    *time.Time
	CompletedAt  *time.Time
}

// Requirement is a constraint a candidate must satisfy — a capability, a
// minimum capacity, a vehicle type.
type Requirement struct {
	Name  string
	Value string
}

// Quote is a price offered for a job.
//
// It holds no pricing logic: this phase creates the record, and CAP-1's
// boundary is created by the ride slice. Amounts are money.Amount, so nothing
// here can be a float.
type Quote struct {
	ID        string
	JobType   Type
	Amount    money.Amount
	Low       *money.Amount
	High      *money.Amount
	Breakdown map[string]any
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// Job is a unit of operational work.
type Job struct {
	ID                string
	Type              Type
	RequesterUserID   string
	MerchantID        string
	Status            Status
	ScheduledAt       *time.Time
	QuoteID           string
	AssignedDriverID  string
	AssignedVehicleID string
	Stops             []Stop
	Requirements      []Requirement
	TerminatedAt      *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Live reports whether the job is still moving through the operational flow.
//
// COMPLETED is deliberately not live even though document 15 does not list it
// as terminal — it is reachable only by DISPUTED, so work on it is over. The
// distinction matters: dispatch and tracking ask "is this still running?", and
// answering yes for a finished trip would keep a driver marked busy.
func (j Job) Live() bool { return !j.Finished() }

// Finished reports whether the job has stopped progressing — terminal, or
// completed and awaiting only a possible dispute.
func (j Job) Finished() bool {
	return Machine.Terminal(j.Status) || j.Status == StatusCompleted
}

// Pickup returns the first PICKUP stop.
func (j Job) Pickup() (Stop, bool) {
	for _, stop := range j.Stops {
		if stop.Type == StopPickup {
			return stop, true
		}
	}
	return Stop{}, false
}

// Dropoff returns the last DROPOFF stop — the final destination of a
// multi-stop job.
func (j Job) Dropoff() (Stop, bool) {
	for i := len(j.Stops) - 1; i >= 0; i-- {
		if j.Stops[i].Type == StopDropoff {
			return j.Stops[i], true
		}
	}
	return Stop{}, false
}

// ActorType is who caused a transition (document 15: "actor").
type ActorType string

const (
	ActorCustomer ActorType = "CUSTOMER"
	ActorDriver   ActorType = "DRIVER"
	ActorMerchant ActorType = "MERCHANT"
	ActorAdmin    ActorType = "ADMIN"
	ActorSupport  ActorType = "SUPPORT"
	ActorSystem   ActorType = "SYSTEM"
)

// Actor identifies who performed a transition.
type Actor struct {
	Type ActorType
	ID   string
}

// StatusChange is one recorded transition — the durable half of document 15's
// JobStatusChanged.
type StatusChange struct {
	ID        int64
	JobID     string
	From      Status
	To        Status
	Actor     Actor
	Metadata  map[string]any
	CreatedAt time.Time
}

// AssignmentStatus is the lifecycle of one offer to one driver.
type AssignmentStatus string

const (
	AssignmentOffered   AssignmentStatus = "OFFERED"
	AssignmentAccepted  AssignmentStatus = "ACCEPTED"
	AssignmentRejected  AssignmentStatus = "REJECTED"
	AssignmentExpired   AssignmentStatus = "EXPIRED"
	AssignmentCancelled AssignmentStatus = "CANCELLED"
	AssignmentCompleted AssignmentStatus = "COMPLETED"
)

// AssignmentMachine governs one offer. An offer is answered, times out, or is
// withdrawn; it is never re-offered in place — a second attempt is a second
// assignment row, which is what makes the dispatch history readable.
var AssignmentMachine = statemachine.New(statemachine.Definition[AssignmentStatus]{
	Name:    "assignment",
	Initial: AssignmentOffered,
	Transitions: map[AssignmentStatus][]AssignmentStatus{
		AssignmentOffered:  {AssignmentAccepted, AssignmentRejected, AssignmentExpired, AssignmentCancelled},
		AssignmentAccepted: {AssignmentCompleted, AssignmentCancelled},
	},
	Terminal: []AssignmentStatus{AssignmentRejected, AssignmentExpired, AssignmentCancelled, AssignmentCompleted},
})

// Assignment is one job offered to one driver.
type Assignment struct {
	ID          string
	JobID       string
	DriverID    string
	VehicleID   string
	Status      AssignmentStatus
	OfferedAt   time.Time
	RespondedAt *time.Time
	AcceptedAt  *time.Time
	CompletedAt *time.Time
	ExpiresAt   *time.Time
}

// Live reports whether this assignment still holds a claim on the job.
func (a Assignment) Live() bool {
	return a.Status == AssignmentOffered || a.Status == AssignmentAccepted
}

var (
	ErrNotFound        = errors.New("jobs: job not found")
	ErrInvalidType     = errors.New("jobs: unknown job type")
	ErrNoStops         = errors.New("jobs: a job needs at least one stop")
	ErrStopOrder       = errors.New("jobs: stop sequences must start at 0 and not repeat")
	ErrBadCoordinate   = errors.New("jobs: coordinate is out of range")
	ErrAlreadyClaimed  = errors.New("jobs: the job already has a live assignment")
	ErrStaleTransition = errors.New("jobs: the job changed since it was read")
)

// ValidateForCreation checks the invariants a job must satisfy before it can be
// stored. Document 04 fixes the shape; this refuses anything outside it.
func (j Job) ValidateForCreation() error {
	if !j.Type.Valid() {
		return ErrInvalidType
	}
	if len(j.Stops) == 0 {
		return ErrNoStops
	}
	seen := map[int]bool{}
	for _, stop := range j.Stops {
		if !stop.Location.Valid() {
			return ErrBadCoordinate
		}
		if seen[stop.Sequence] {
			return ErrStopOrder
		}
		seen[stop.Sequence] = true
	}
	// Sequences must be dense from zero: a gap means a stop was dropped
	// somewhere between the client and here, and the route is not what anyone
	// thinks it is.
	for i := 0; i < len(j.Stops); i++ {
		if !seen[i] {
			return ErrStopOrder
		}
	}
	return nil
}
