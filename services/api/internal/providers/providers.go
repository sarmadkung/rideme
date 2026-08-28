// Package providers is the supply side of the platform: drivers, their
// vehicles, their documents, and the verification that gates all three
// (documents 16, 29, 30, 108).
//
// Document 29's objective states the separation this package keeps: "Allow a
// user to become a verified driver without mixing verification state with
// availability state." They are two columns and two machines. A driver who is
// approved but offline is not the same as one who is online but unverified,
// and collapsing them makes both unanswerable.
package providers

import (
	"errors"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/statemachine"
)

// VerificationStatus is a driver's progress through onboarding (document 29).
type VerificationStatus string

const (
	VerificationNotStarted  VerificationStatus = "NOT_STARTED"
	VerificationInProgress  VerificationStatus = "IN_PROGRESS"
	VerificationSubmitted   VerificationStatus = "SUBMITTED"
	VerificationUnderReview VerificationStatus = "UNDER_REVIEW"
	VerificationApproved    VerificationStatus = "APPROVED"
	VerificationRejected    VerificationStatus = "REJECTED"
	VerificationSuspended   VerificationStatus = "SUSPENDED"
)

// VerificationMachine follows document 29's flow. Rejection is not terminal —
// the same document requires that "rejected documents can be resubmitted", so
// a rejected driver must be able to re-enter the funnel. A suspension is
// lifted by review, not by resubmission.
var VerificationMachine = statemachine.New(statemachine.Definition[VerificationStatus]{
	Name:    "driver_verification",
	Initial: VerificationNotStarted,
	Transitions: map[VerificationStatus][]VerificationStatus{
		VerificationNotStarted:  {VerificationInProgress},
		VerificationInProgress:  {VerificationSubmitted},
		VerificationSubmitted:   {VerificationUnderReview, VerificationRejected},
		VerificationUnderReview: {VerificationApproved, VerificationRejected},
		VerificationApproved:    {VerificationSuspended, VerificationUnderReview},
		VerificationRejected:    {VerificationInProgress},
		VerificationSuspended:   {VerificationUnderReview, VerificationApproved},
	},
})

// AvailabilityStatus is a driver's operational state (document 16).
type AvailabilityStatus string

const (
	StatusOffline   AvailabilityStatus = "OFFLINE"
	StatusAvailable AvailabilityStatus = "AVAILABLE"
	StatusOffered   AvailabilityStatus = "OFFERED"
	StatusAccepted  AvailabilityStatus = "ACCEPTED"
	StatusOnTrip    AvailabilityStatus = "ON_TRIP"
	StatusPaused    AvailabilityStatus = "PAUSED"
	StatusSuspended AvailabilityStatus = "SUSPENDED"
	StatusBlocked   AvailabilityStatus = "BLOCKED"
)

// AvailabilityMachine is document 16's cycle:
//
//	OFFLINE -> AVAILABLE -> OFFERED -> ACCEPTED -> ON_TRIP -> AVAILABLE
//
// with PAUSED, SUSPENDED and BLOCKED alongside. OFFERED returns to AVAILABLE
// when a driver declines or the offer times out — without that edge a single
// unanswered offer would strand the driver.
var AvailabilityMachine = statemachine.New(statemachine.Definition[AvailabilityStatus]{
	Name:    "driver_availability",
	Initial: StatusOffline,
	Transitions: map[AvailabilityStatus][]AvailabilityStatus{
		StatusOffline:   {StatusAvailable, StatusSuspended, StatusBlocked},
		StatusAvailable: {StatusOffered, StatusPaused, StatusOffline, StatusSuspended, StatusBlocked},
		StatusOffered:   {StatusAccepted, StatusAvailable, StatusOffline, StatusSuspended, StatusBlocked},
		StatusAccepted:  {StatusOnTrip, StatusAvailable, StatusSuspended, StatusBlocked},
		StatusOnTrip:    {StatusAvailable, StatusSuspended, StatusBlocked},
		StatusPaused:    {StatusAvailable, StatusOffline, StatusSuspended, StatusBlocked},
		StatusSuspended: {StatusOffline, StatusBlocked},
		StatusBlocked:   {StatusOffline},
	},
})

// VehicleStatus is a vehicle's verification state (document 30).
type VehicleStatus string

const (
	VehiclePending     VehicleStatus = "PENDING"
	VehicleUnderReview VehicleStatus = "UNDER_REVIEW"
	VehicleVerified    VehicleStatus = "VERIFIED"
	VehicleRejected    VehicleStatus = "REJECTED"
	VehicleSuspended   VehicleStatus = "SUSPENDED"
	VehicleExpired     VehicleStatus = "EXPIRED"
)

var VehicleMachine = statemachine.New(statemachine.Definition[VehicleStatus]{
	Name:    "vehicle_verification",
	Initial: VehiclePending,
	Transitions: map[VehicleStatus][]VehicleStatus{
		VehiclePending:     {VehicleUnderReview, VehicleRejected},
		VehicleUnderReview: {VehicleVerified, VehicleRejected},
		// A verified vehicle lapses when a mandatory document expires, and
		// returns through review rather than straight back to verified.
		VehicleVerified:  {VehicleSuspended, VehicleExpired, VehicleUnderReview},
		VehicleRejected:  {VehicleUnderReview},
		VehicleSuspended: {VehicleUnderReview, VehicleVerified},
		VehicleExpired:   {VehicleUnderReview},
	},
})

// DocumentStatus is a credential's review state (document 29).
type DocumentStatus string

const (
	DocumentPending  DocumentStatus = "PENDING"
	DocumentVerified DocumentStatus = "VERIFIED"
	DocumentRejected DocumentStatus = "REJECTED"
	DocumentExpired  DocumentStatus = "EXPIRED"
)

// Driver is a user who may take work.
type Driver struct {
	ID                 string
	UserID             string
	VerificationStatus VerificationStatus
	Status             AvailabilityStatus
	ActiveVehicleID    string
	Rating             *float64
	CompletionRate     *float64
	CancellationRate   *float64
	AcceptanceRate     *float64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CanGoOnline reports whether the driver has cleared verification. It is not
// the whole gate — eligibility also checks the vehicle and the documents.
func (d Driver) CanGoOnline() bool { return d.VerificationStatus == VerificationApproved }

// Vehicle is a registered vehicle.
type Vehicle struct {
	ID                 string
	OwnerUserID        string
	Type               string
	Make               string
	Model              string
	Year               *int
	Color              string
	PlateNumber        string
	CapacityKG         *float64
	Dimensions         map[string]any
	VerificationStatus VehicleStatus
	Capabilities       []Capability
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CapabilitySource records where a capability came from. Document 30: "Do not
// trust capabilities submitted by the client. Backend determines final
// eligibility." A client-asserted capability is not representable — the only
// sources are derivation from the vehicle type and an explicit admin grant.
type CapabilitySource string

const (
	SourceDerived CapabilitySource = "DERIVED"
	SourceAdmin   CapabilitySource = "ADMIN"
)

type Capability struct {
	Code   string
	Source CapabilitySource
}

// Document is a credential belonging to a driver or a vehicle.
type Document struct {
	ID              string
	OwnerType       string
	DriverID        string
	VehicleID       string
	Type            string
	Number          string
	FileKey         string
	IssuedAt        *time.Time
	ExpiresAt       *time.Time
	Status          DocumentStatus
	RejectionReason string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Expired reports whether the document has lapsed as of now.
func (d Document) Expired(now time.Time) bool {
	return d.ExpiresAt != nil && !now.Before(*d.ExpiresAt)
}

// Requirement is a mandatory document for a market and vehicle type.
//
// The rows are configuration, and the table ships empty: BD-14 is a regulatory
// decision about which licences and permits each vehicle type needs, and it is
// not the platform's to make. The mechanism works with zero rows — it simply
// requires nothing until the market's list is supplied.
type Requirement struct {
	Market      string
	OwnerType   string
	VehicleType string
	Type        string
	Mandatory   bool
}

var (
	ErrDriverNotFound  = errors.New("providers: driver not found")
	ErrVehicleNotFound = errors.New("providers: vehicle not found")
	ErrNotOwner        = errors.New("providers: the vehicle belongs to someone else")
	ErrVehicleNotReady = errors.New("providers: the vehicle is not verified")
	ErrStale           = errors.New("providers: the record changed since it was read")
)
