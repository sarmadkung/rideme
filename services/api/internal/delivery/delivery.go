// Package delivery covers the non-passenger services: parcel and cargo
// (documents 79–91).
//
// It adds three things to the job core, and nothing else: evidence that a
// delivery happened, a deterministic path when one did not, and the physical
// constraints a cargo vehicle must satisfy. Parcel and cargo remain Jobs with a
// type — there is no parallel order entity here, and the pricing they use is
// CAP-1's engine with two more rule sets registered.
package delivery

import (
	"errors"
	"fmt"
	"time"
)

// ProofMethod is how a delivery was evidenced (document 83).
type ProofMethod string

const (
	ProofRecipientOTP          ProofMethod = "RECIPIENT_OTP"
	ProofSignature             ProofMethod = "SIGNATURE"
	ProofPhoto                 ProofMethod = "PHOTO"
	ProofRecipientConfirmation ProofMethod = "RECIPIENT_CONFIRMATION"
	ProofCodeScan              ProofMethod = "CODE_SCAN"
	ProofMerchantConfirmation  ProofMethod = "MERCHANT_CONFIRMATION"
)

var AllProofMethods = []ProofMethod{
	ProofRecipientOTP, ProofSignature, ProofPhoto,
	ProofRecipientConfirmation, ProofCodeScan, ProofMerchantConfirmation,
}

func (m ProofMethod) Valid() bool {
	for _, known := range AllProofMethods {
		if m == known {
			return true
		}
	}
	return false
}

// Proof is a delivery evidence record.
type Proof struct {
	ID            string
	JobID         string
	StopID        string
	Method        ProofMethod
	MediaKey      string
	Lat, Lon      float64
	HasLocation   bool
	RecipientName string
	Verified      bool
	CapturedBy    string
	Metadata      map[string]any
	CreatedAt     time.Time
}

// FailureReason is why a delivery attempt did not succeed (document 84).
type FailureReason string

const (
	FailureRecipientUnavailable FailureReason = "RECIPIENT_UNAVAILABLE"
	FailureWrongAddress         FailureReason = "WRONG_ADDRESS"
	FailureRecipientRejected    FailureReason = "RECIPIENT_REJECTED"
	FailureDamagedPackage       FailureReason = "DAMAGED_PACKAGE"
	FailureAccessBlocked        FailureReason = "ACCESS_BLOCKED"
	FailureMerchantIssue        FailureReason = "MERCHANT_ISSUE"
)

// NextAction is what happens after a failure. Document 84 requires the answer
// be deterministic — a parcel with no defined next action is a parcel nobody
// owns.
type NextAction string

const (
	ActionRetry      NextAction = "RETRY"
	ActionReschedule NextAction = "RESCHEDULE"
	ActionReturn     NextAction = "RETURN"
	ActionEscalate   NextAction = "ESCALATE"
)

// RetryPolicy bounds delivery attempts. Document 84: "Retry limits should be
// configurable." The values are engineering defaults; what a failed delivery
// *costs* is BD-10 and unresolved.
type RetryPolicy struct {
	MaxAttempts int
}

func DefaultRetryPolicy() RetryPolicy { return RetryPolicy{MaxAttempts: 3} }

// DecideNextAction maps a failure to its next action.
//
// The mapping is a policy decision the documentation makes only by example, so
// it is expressed here as one readable function rather than scattered through
// handlers. Two of the reasons never warrant a retry, and saying so once is
// what keeps a driver from being sent back to an address that does not exist.
func DecideNextAction(reason FailureReason, attempt int, policy RetryPolicy) NextAction {
	switch reason {
	case FailureWrongAddress:
		// Retrying the same wrong address produces the same failure.
		return ActionEscalate
	case FailureRecipientRejected:
		// The recipient said no; the parcel goes back.
		return ActionReturn
	case FailureDamagedPackage, FailureMerchantIssue:
		// Neither is the driver's to resolve on the doorstep.
		return ActionEscalate
	case FailureRecipientUnavailable, FailureAccessBlocked:
		if attempt < policy.MaxAttempts {
			return ActionRetry
		}
		return ActionReturn
	default:
		return ActionEscalate
	}
}

// CustomerMessage renders a failure for the customer.
//
// Document 84: "Use customer-safe messages and avoid exposing internal failure
// codes." A customer told MERCHANT_ISSUE learns nothing and is alarmed by it.
func CustomerMessage(reason FailureReason) string {
	switch reason {
	case FailureRecipientUnavailable:
		return "Nobody was available to receive the delivery."
	case FailureWrongAddress:
		return "We could not find the delivery address."
	case FailureRecipientRejected:
		return "The delivery was declined at the door."
	case FailureDamagedPackage:
		return "There was a problem with the package."
	case FailureAccessBlocked:
		return "The driver could not reach the delivery point."
	case FailureMerchantIssue:
		return "There was a problem with the order."
	default:
		return "The delivery could not be completed."
	}
}

// Attempt is one delivery attempt at one stop.
type Attempt struct {
	ID         string
	JobID      string
	StopID     string
	Attempt    int
	Delivered  bool
	Reason     FailureReason
	NextAction NextAction
	Notes      string
	CreatedAt  time.Time
}

// LoadingAssistance is who does the lifting (document 87).
type LoadingAssistance string

const (
	AssistDriverOnly       LoadingAssistance = "DRIVER_ONLY"
	AssistDriverPlusHelper LoadingAssistance = "DRIVER_PLUS_HELPER"
	AssistCustomer         LoadingAssistance = "CUSTOMER_LOADING"
	AssistMerchant         LoadingAssistance = "MERCHANT_LOADING"
)

// Cargo is the physical description of a load (document 80).
type Cargo struct {
	JobID                string
	TotalWeightKG        *float64
	LengthCM             *float64
	WidthCM              *float64
	HeightCM             *float64
	VolumeM3             *float64
	ItemCount            *int
	Fragile              bool
	TemperatureSensitive bool
	SpecialHandling      string
	LoadingAssistance    LoadingAssistance
}

// VehicleCapacity is what a vehicle can physically carry.
type VehicleCapacity struct {
	MaxWeightKG   *float64
	MaxVolumeM3   *float64
	CargoLengthCM *float64
	CargoWidthCM  *float64
	CargoHeightCM *float64
	Equipment     []string
}

// CapacityFailure names an unmet physical constraint.
type CapacityFailure string

const (
	CapacityWeight    CapacityFailure = "weight_exceeded"
	CapacityVolume    CapacityFailure = "volume_exceeded"
	CapacityLength    CapacityFailure = "length_exceeded"
	CapacityWidth     CapacityFailure = "width_exceeded"
	CapacityHeight    CapacityFailure = "height_exceeded"
	CapacityUnknown   CapacityFailure = "vehicle_capacity_unknown"
	CapacityEquipment CapacityFailure = "equipment_missing"
)

// Fits reports whether a vehicle can carry a load, and why not if it cannot.
//
// Document 80's hard constraints, and document 41's instruction: "Do not use
// weight alone." A 3-metre pipe weighing 20kg fits a motorcycle by weight and
// by nothing else.
//
// An unknown vehicle capacity fails rather than passes. The alternative is
// dispatching a load to a vehicle that may not carry it, which is discovered
// at the pickup by a driver who cannot do the job.
func Fits(cargo Cargo, vehicle VehicleCapacity, requiredEquipment []string) []CapacityFailure {
	var failures []CapacityFailure

	exceeds := func(required, capacity *float64, unknown, over CapacityFailure) {
		if required == nil {
			return
		}
		if capacity == nil {
			failures = append(failures, unknown)
			return
		}
		if *required > *capacity {
			failures = append(failures, over)
		}
	}

	exceeds(cargo.TotalWeightKG, vehicle.MaxWeightKG, CapacityUnknown, CapacityWeight)
	exceeds(cargo.VolumeM3, vehicle.MaxVolumeM3, CapacityUnknown, CapacityVolume)
	exceeds(cargo.LengthCM, vehicle.CargoLengthCM, CapacityUnknown, CapacityLength)
	exceeds(cargo.WidthCM, vehicle.CargoWidthCM, CapacityUnknown, CapacityWidth)
	exceeds(cargo.HeightCM, vehicle.CargoHeightCM, CapacityUnknown, CapacityHeight)

	for _, required := range requiredEquipment {
		found := false
		for _, held := range vehicle.Equipment {
			if held == required {
				found = true
				break
			}
		}
		if !found {
			failures = append(failures, CapacityEquipment)
			break
		}
	}
	return failures
}

// Timing records document 87's stop timestamps.
type Timing struct {
	StopID                string
	JobID                 string
	ArrivedAt             *time.Time
	WaitingStartedAt      *time.Time
	LoadingStartedAt      *time.Time
	LoadedAt              *time.Time
	UnloadingStartedAt    *time.Time
	UnloadedAt            *time.Time
	GraceSeconds          int
	ChargeableWaitingSecs int
	LoadingSeconds        int
}

// ChargeableWaiting computes billable waiting after the grace period.
//
// Recording the time and pricing it are separate: BD-13 leaves the rate open,
// and this returns seconds, never money. A tariff with a zero rate prices it at
// nothing without losing the record.
func ChargeableWaiting(arrivedAt, endedAt time.Time, graceSeconds int) int {
	if arrivedAt.IsZero() || endedAt.IsZero() || !endedAt.After(arrivedAt) {
		return 0
	}
	waited := int(endedAt.Sub(arrivedAt).Seconds())
	if waited <= graceSeconds {
		return 0
	}
	return waited - graceSeconds
}

var (
	ErrInvalidMethod     = errors.New("delivery: unknown proof method")
	ErrProofRequired     = errors.New("delivery: this stop requires proof before completion")
	ErrOTPIncorrect      = errors.New("delivery: the recipient code is incorrect or has expired")
	ErrAttemptsExhausted = errors.New("delivery: no delivery attempts remain")
	ErrRestrictedGoods   = errors.New("delivery: the load contains restricted goods")
)

// RequiredProof reports which proof method a job type needs.
//
// A ride needs none — the passenger got out. A parcel needs evidence it reached
// someone. Document 83 makes the method configurable; this is the default when
// no configuration exists, and it is deliberately not "none".
func RequiredProof(jobType string) (ProofMethod, bool) {
	switch jobType {
	case "PARCEL":
		return ProofRecipientOTP, true
	case "GROCERY":
		return ProofRecipientConfirmation, true
	case "CARGO", "FREIGHT":
		return ProofSignature, true
	default:
		return "", false
	}
}

// ValidateProof checks a proof record is usable before it is stored.
func ValidateProof(p Proof) error {
	if !p.Method.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidMethod, p.Method)
	}
	switch p.Method {
	case ProofPhoto, ProofSignature:
		if p.MediaKey == "" {
			return fmt.Errorf("delivery: %s proof requires a media reference", p.Method)
		}
	}
	return nil
}
