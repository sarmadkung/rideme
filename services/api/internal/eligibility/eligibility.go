// Package eligibility decides whether a driver and vehicle may perform a job.
//
// There is exactly one implementation of these rules, and this is it.
//
// That is the point of the package. Document 41 ends with: "No dispatch path
// can bypass hard capability constraints." The way that requirement fails in
// practice is not that someone writes a bypass — it is that dispatch filters
// candidates with one copy of the rules and job acceptance re-checks them with
// another, the two drift by one condition, and a truck-sized load is offered to
// a motorcycle by the path nobody updated. Dispatch (Phase 8) and acceptance
// both call Evaluate; neither has its own copy.
//
// Hard constraints reject a candidate outright. Soft preferences are not here:
// they influence the dispatch score (document 05), and mixing "cannot" with
// "would rather not" is how a preference silently becomes a rule.
package eligibility

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reason is a machine-readable explanation for a refusal, so an operator
// console can say why a driver saw no work rather than only that they did not.
type Reason string

const (
	ReasonDriverNotApproved   Reason = "driver_not_approved"
	ReasonDriverUnavailable   Reason = "driver_unavailable"
	ReasonNoActiveVehicle     Reason = "no_active_vehicle"
	ReasonVehicleNotVerified  Reason = "vehicle_not_verified"
	ReasonCapabilityMissing   Reason = "capability_missing"
	ReasonCapacityExceeded    Reason = "capacity_exceeded"
	ReasonPassengersExceeded  Reason = "passengers_exceeded"
	ReasonDocumentExpired     Reason = "document_expired"
	ReasonDocumentMissing     Reason = "document_missing"
	ReasonVehicleTypeExcluded Reason = "vehicle_type_excluded"
	ReasonLocationStale       Reason = "location_stale"
)

// Failure is one unmet hard constraint.
type Failure struct {
	Reason Reason
	Detail string
}

func (f Failure) String() string {
	if f.Detail == "" {
		return string(f.Reason)
	}
	return string(f.Reason) + ": " + f.Detail
}

// Decision is the outcome. It carries every failure rather than the first, so
// a driver fixing their profile learns all of what is wrong in one pass.
type Decision struct {
	Eligible bool
	Failures []Failure
}

func (d Decision) Error() string {
	if d.Eligible {
		return ""
	}
	parts := make([]string, len(d.Failures))
	for i, f := range d.Failures {
		parts[i] = f.String()
	}
	return "ineligible: " + strings.Join(parts, "; ")
}

// Document is a credential with an optional expiry.
type Document struct {
	Type      string
	Status    string
	ExpiresAt *time.Time
	Mandatory bool
}

// Expired reports whether a mandatory document has lapsed.
func (d Document) Expired(now time.Time) bool {
	return d.ExpiresAt != nil && !now.Before(*d.ExpiresAt)
}

// Driver is the candidate's provider-side state.
type Driver struct {
	ID                 string
	VerificationStatus string
	Status             string
	Documents          []Document
	// LocationAt is when the driver's position was last known. Document 16:
	// "Dispatch excludes stale locations."
	LocationAt *time.Time
}

// Vehicle is the candidate's vehicle-side state.
type Vehicle struct {
	ID                 string
	Type               string
	VerificationStatus string
	Capabilities       []string
	CapacityKG         *float64
	PassengerSeats     *int
	Documents          []Document
}

func (v Vehicle) HasCapability(capability string) bool {
	for _, held := range v.Capabilities {
		if held == capability {
			return true
		}
	}
	return false
}

// Requirements are what a job demands of a candidate. They come from the job's
// type and its job_requirements rows, never from the client directly —
// document 30: "Do not trust capabilities submitted by the client."
type Requirements struct {
	// Capability the job needs — PASSENGER for a ride, HEAVY_CARGO for a haul.
	Capability string
	// WeightKG is the load. Document 41 requires capacity to be checked
	// against it rather than assumed.
	WeightKG *float64
	// Passengers for ride jobs. Document 41: passenger_count <=
	// passenger_capacity is a safety constraint, not a preference.
	Passengers *int
	// VehicleTypes restricts the job to specific types when a customer or a
	// market rule demands one. Empty means any type that has the capability.
	VehicleTypes []string
}

// Options tune the evaluation for its caller.
type Options struct {
	// Now is the clock; zero means time.Now.
	Now time.Time
	// MaxLocationAge rejects a driver whose position is too old to dispatch
	// against. Zero disables the check — correct for acceptance, where the
	// driver is present and answering, and wrong for dispatch, which is
	// choosing between candidates it cannot see.
	MaxLocationAge time.Duration
	// RequireAvailable demands the driver be AVAILABLE. Dispatch needs this;
	// acceptance does not, because a driver responding to an offer is already
	// OFFERED rather than AVAILABLE.
	RequireAvailable bool
}

// Available driver states. Document 16's machine:
// OFFLINE -> AVAILABLE -> OFFERED -> ACCEPTED -> ON_TRIP -> AVAILABLE.
const (
	DriverAvailable = "AVAILABLE"
	DriverOffered   = "OFFERED"
	DriverAccepted  = "ACCEPTED"
	DriverOnTrip    = "ON_TRIP"
)

// Evaluate applies every hard constraint in document 41 and returns all
// failures.
func Evaluate(driver Driver, vehicle Vehicle, req Requirements, opts Options) Decision {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	var failures []Failure
	fail := func(reason Reason, format string, args ...any) {
		detail := ""
		if format != "" {
			detail = fmt.Sprintf(format, args...)
		}
		failures = append(failures, Failure{Reason: reason, Detail: detail})
	}

	// --- driver ---

	if driver.VerificationStatus != "APPROVED" {
		fail(ReasonDriverNotApproved, "driver verification is %s", driver.VerificationStatus)
	}
	if opts.RequireAvailable && driver.Status != DriverAvailable {
		fail(ReasonDriverUnavailable, "driver is %s", driver.Status)
	} else if !opts.RequireAvailable {
		// Even when availability is not required, a suspended or blocked
		// driver may never take work.
		switch driver.Status {
		case DriverAvailable, DriverOffered, DriverAccepted, DriverOnTrip:
		default:
			fail(ReasonDriverUnavailable, "driver is %s", driver.Status)
		}
	}
	if opts.MaxLocationAge > 0 {
		if driver.LocationAt == nil {
			fail(ReasonLocationStale, "no known location")
		} else if now.Sub(*driver.LocationAt) > opts.MaxLocationAge {
			fail(ReasonLocationStale, "location is %s old", now.Sub(*driver.LocationAt).Round(time.Second))
		}
	}
	checkDocuments(driver.Documents, now, &failures, "driver")

	// --- vehicle ---

	if vehicle.ID == "" {
		// Document 30: a driver selects one active vehicle before going
		// online. Without it there is nothing to check capability against.
		fail(ReasonNoActiveVehicle, "")
		return decide(failures)
	}
	if vehicle.VerificationStatus != "VERIFIED" {
		fail(ReasonVehicleNotVerified, "vehicle verification is %s", vehicle.VerificationStatus)
	}
	checkDocuments(vehicle.Documents, now, &failures, "vehicle")

	// --- job requirements ---

	if req.Capability != "" && !vehicle.HasCapability(req.Capability) {
		fail(ReasonCapabilityMissing, "vehicle cannot %s", req.Capability)
	}
	if len(req.VehicleTypes) > 0 {
		allowed := false
		for _, t := range req.VehicleTypes {
			if t == vehicle.Type {
				allowed = true
				break
			}
		}
		if !allowed {
			sorted := append([]string(nil), req.VehicleTypes...)
			sort.Strings(sorted)
			fail(ReasonVehicleTypeExcluded, "%s is not one of %s", vehicle.Type, strings.Join(sorted, ", "))
		}
	}
	if req.WeightKG != nil {
		// An unknown capacity is not an unlimited one. Refusing is the safe
		// reading: the alternative sends a load to a vehicle that may not
		// carry it.
		if vehicle.CapacityKG == nil {
			fail(ReasonCapacityExceeded, "vehicle capacity is unknown")
		} else if *req.WeightKG > *vehicle.CapacityKG {
			fail(ReasonCapacityExceeded, "%.1fkg exceeds %.1fkg",
				*req.WeightKG, *vehicle.CapacityKG)
		}
	}
	if req.Passengers != nil {
		if vehicle.PassengerSeats == nil {
			fail(ReasonPassengersExceeded, "vehicle passenger capacity is unknown")
		} else if *req.Passengers > *vehicle.PassengerSeats {
			fail(ReasonPassengersExceeded, "%d passengers exceed %d seats",
				*req.Passengers, *vehicle.PassengerSeats)
		}
	}

	return decide(failures)
}

func checkDocuments(docs []Document, now time.Time, failures *[]Failure, owner string) {
	for _, doc := range docs {
		if !doc.Mandatory {
			continue
		}
		switch {
		case doc.Status == "EXPIRED" || doc.Expired(now):
			// Document 16: a driver cannot accept jobs with expired required
			// documents. This is the gate.
			*failures = append(*failures, Failure{
				Reason: ReasonDocumentExpired,
				Detail: owner + " " + doc.Type,
			})
		case doc.Status != "VERIFIED":
			*failures = append(*failures, Failure{
				Reason: ReasonDocumentMissing,
				Detail: owner + " " + doc.Type + " is " + strings.ToLower(doc.Status),
			})
		}
	}
}

func decide(failures []Failure) Decision {
	if len(failures) == 0 {
		return Decision{Eligible: true}
	}
	return Decision{Eligible: false, Failures: failures}
}

// RequirementsFromJob turns a job type and its stored requirement rows into
// Requirements.
//
// It is here rather than in the jobs package so that the translation from
// stored rows to constraints happens once, next to the rules that consume it.
func RequirementsFromJob(jobType string, rows map[string]string) Requirements {
	req := Requirements{Capability: defaultCapability(jobType)}

	if raw, ok := rows["capability"]; ok && raw != "" {
		req.Capability = raw
	}
	if raw, ok := rows["weight_kg"]; ok {
		if weight, err := strconv.ParseFloat(raw, 64); err == nil {
			req.WeightKG = &weight
		}
	}
	if raw, ok := rows["passengers"]; ok {
		if count, err := strconv.Atoi(raw); err == nil {
			req.Passengers = &count
		}
	}
	if raw, ok := rows["vehicle_types"]; ok && raw != "" {
		for _, t := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				req.VehicleTypes = append(req.VehicleTypes, trimmed)
			}
		}
	}
	return req
}

// defaultCapability maps a job type to the capability it needs when the job
// carries no explicit one.
func defaultCapability(jobType string) string {
	switch jobType {
	case "RIDE":
		return "PASSENGER"
	case "PARCEL":
		return "PARCEL"
	case "GROCERY":
		return "GROCERY"
	case "CARGO", "FREIGHT":
		// The conservative default. A cargo job that knows its weight can ask
		// for SMALL_CARGO explicitly; one that does not should not be offered
		// to a vehicle sized for parcels.
		return "HEAVY_CARGO"
	default:
		return ""
	}
}
