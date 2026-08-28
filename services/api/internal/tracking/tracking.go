// Package tracking owns driver location: ingestion, validation, current state
// and durable history (documents 18, 48, 98, 102).
//
// The pipeline document 48 draws ends at "Redis Current State → Dispatch /
// Tracking", and document 13 keeps PostgreSQL as the durable source of truth.
// Both are true and they answer different questions: dispatch asks "where is
// every available driver, now", which is a geospatial query over volatile data;
// an investigation asks "where was this driver at 14:20 last Tuesday", which is
// history. Redis serves the first, Postgres the second.
package tracking

import (
	"errors"
	"fmt"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

// Fix is one location report from a device. Document 48 fixes the payload.
type Fix struct {
	DriverID   string
	VehicleID  string
	JobID      string
	Lat        float64
	Lon        float64
	AccuracyM  *float64
	HeadingDeg *float64
	SpeedMPS   *float64
	RecordedAt time.Time
}

func (f Fix) Point() routing.Point { return routing.Point{Lat: f.Lat, Lon: f.Lon} }

// Rejection explains why a fix was discarded.
type Rejection string

const (
	RejectBadCoordinate   Rejection = "impossible_coordinate"
	RejectFutureTimestamp Rejection = "future_timestamp"
	RejectStaleTimestamp  Rejection = "stale_timestamp"
	RejectImpossibleSpeed Rejection = "impossible_speed"
	RejectImpossibleJump  Rejection = "impossible_jump"
	RejectPoorAccuracy    Rejection = "poor_accuracy"
	RejectOutOfOrder      Rejection = "out_of_order"
)

// ErrRejected wraps a validation failure.
type ErrRejected struct {
	Reason Rejection
	Detail string
}

func (e *ErrRejected) Error() string {
	if e.Detail == "" {
		return "tracking: " + string(e.Reason)
	}
	return "tracking: " + string(e.Reason) + ": " + e.Detail
}

// Is lets callers match on the reason.
func (e *ErrRejected) Is(target error) bool {
	other, ok := target.(*ErrRejected)
	return ok && other.Reason == e.Reason
}

// Limits are the validation thresholds document 48 requires without stating
// values. They are engineering defaults, tunable from configuration, and
// deliberately generous: a false rejection makes a driver invisible to
// dispatch, which costs a job, while a false acceptance costs one bad point
// that the next fix corrects.
type Limits struct {
	// MaxClockSkew tolerates devices whose clocks run slightly fast. A fix
	// timestamped further ahead than this is not a fix, it is a bug or a spoof.
	MaxClockSkew time.Duration
	// MaxAge discards reports too old to be operationally useful. Document 16
	// requires dispatch to exclude stale locations; this is where "stale" is
	// decided at ingestion.
	MaxAge time.Duration
	// MaxSpeedMPS is roughly 200 km/h. Faster than any vehicle in the fleet
	// travels, and one of the GPS-spoofing signals document 20 names.
	MaxSpeedMPS float64
	// MaxAccuracyM rejects fixes too imprecise to act on. A 500-metre radius
	// in a city is a different street.
	MaxAccuracyM float64
	// MaxJumpSpeedMPS bounds the implied speed between consecutive fixes.
	// Higher than MaxSpeedMPS because a tunnel exit legitimately produces one
	// large gap, and rejecting that would blind dispatch to a driver who is
	// simply back on the network.
	MaxJumpSpeedMPS float64
}

func DefaultLimits() Limits {
	return Limits{
		MaxClockSkew:    30 * time.Second,
		MaxAge:          5 * time.Minute,
		MaxSpeedMPS:     55,
		MaxAccuracyM:    500,
		MaxJumpSpeedMPS: 90,
	}
}

// Previous is the last accepted fix for a driver, used for jump detection.
type Previous struct {
	Lat        float64
	Lon        float64
	RecordedAt time.Time
}

// Validate applies document 48's checks: "impossible coordinates, future
// timestamps, stale timestamps, impossible speed, sudden unrealistic jumps".
//
// previous may be nil for a driver's first fix.
func Validate(fix Fix, previous *Previous, limits Limits, now time.Time) error {
	if fix.DriverID == "" {
		return errors.New("tracking: driver is required")
	}
	if !fix.Point().Valid() || (fix.Lat == 0 && fix.Lon == 0) {
		return &ErrRejected{Reason: RejectBadCoordinate,
			Detail: fmt.Sprintf("%.6f,%.6f", fix.Lat, fix.Lon)}
	}
	if fix.RecordedAt.IsZero() {
		return &ErrRejected{Reason: RejectStaleTimestamp, Detail: "no timestamp"}
	}
	if fix.RecordedAt.After(now.Add(limits.MaxClockSkew)) {
		return &ErrRejected{Reason: RejectFutureTimestamp,
			Detail: fix.RecordedAt.Sub(now).Round(time.Second).String() + " ahead"}
	}
	if now.Sub(fix.RecordedAt) > limits.MaxAge {
		return &ErrRejected{Reason: RejectStaleTimestamp,
			Detail: now.Sub(fix.RecordedAt).Round(time.Second).String() + " old"}
	}
	if fix.SpeedMPS != nil && *fix.SpeedMPS > limits.MaxSpeedMPS {
		return &ErrRejected{Reason: RejectImpossibleSpeed,
			Detail: fmt.Sprintf("%.1f m/s", *fix.SpeedMPS)}
	}
	if fix.AccuracyM != nil && *fix.AccuracyM > limits.MaxAccuracyM {
		return &ErrRejected{Reason: RejectPoorAccuracy,
			Detail: fmt.Sprintf("%.0fm", *fix.AccuracyM)}
	}

	if previous != nil {
		if !fix.RecordedAt.After(previous.RecordedAt) {
			// Out-of-order delivery is normal on a buffered pipeline; the
			// older fix is simply not news.
			return &ErrRejected{Reason: RejectOutOfOrder,
				Detail: "not newer than the last accepted fix"}
		}
		elapsed := fix.RecordedAt.Sub(previous.RecordedAt).Seconds()
		if elapsed > 0 {
			distance := routing.HaversineMeters(
				routing.Point{Lat: previous.Lat, Lon: previous.Lon}, fix.Point())
			if implied := distance / elapsed; implied > limits.MaxJumpSpeedMPS {
				return &ErrRejected{Reason: RejectImpossibleJump,
					Detail: fmt.Sprintf("%.0fm in %.0fs implies %.0f m/s", distance, elapsed, implied)}
			}
		}
	}
	return nil
}

// Current is a driver's live position as dispatch sees it.
type Current struct {
	DriverID   string    `json:"driver_id"`
	VehicleID  string    `json:"vehicle_id,omitempty"`
	JobID      string    `json:"job_id,omitempty"`
	Lat        float64   `json:"lat"`
	Lon        float64   `json:"lon"`
	HeadingDeg *float64  `json:"heading_deg,omitempty"`
	SpeedMPS   *float64  `json:"speed_mps,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
}

// Fresh reports whether the position is recent enough to dispatch against
// (document 16: "Dispatch excludes stale locations").
func (c Current) Fresh(now time.Time, maxAge time.Duration) bool {
	return now.Sub(c.RecordedAt) <= maxAge
}

// Session is a tracking window for one job.
type Session struct {
	ID        string
	JobID     string
	DriverID  string
	StartedAt time.Time
	EndedAt   *time.Time
}

func (s Session) Live() bool { return s.EndedAt == nil }

// Scope is who is asking to see a driver's location (document 102).
type Scope string

const (
	ScopeOwnJob      Scope = "own_job"
	ScopeAssignedJob Scope = "assigned_job"
	ScopeMerchant    Scope = "merchant_delivery"
	ScopeOperations  Scope = "operations"
	ScopeSelf        Scope = "self"
)

var ErrNotPermitted = errors.New("tracking: not permitted to view this location")
