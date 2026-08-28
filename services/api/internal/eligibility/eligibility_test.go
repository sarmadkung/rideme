package eligibility_test

import (
	"strings"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/eligibility"
)

var now = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

func approvedDriver() eligibility.Driver {
	seen := now.Add(-10 * time.Second)
	return eligibility.Driver{
		ID:                 "driver-1",
		VerificationStatus: "APPROVED",
		Status:             eligibility.DriverAvailable,
		LocationAt:         &seen,
	}
}

func verifiedBike() eligibility.Vehicle {
	capacity := 20.0
	seats := 1
	return eligibility.Vehicle{
		ID:                 "vehicle-1",
		Type:               "MOTORCYCLE",
		VerificationStatus: "VERIFIED",
		Capabilities:       []string{"PASSENGER", "PARCEL", "GROCERY"},
		CapacityKG:         &capacity,
		PassengerSeats:     &seats,
	}
}

func hasReason(d eligibility.Decision, reason eligibility.Reason) bool {
	for _, f := range d.Failures {
		if f.Reason == reason {
			return true
		}
	}
	return false
}

func TestAnApprovedDriverOnAVerifiedVehicleIsEligible(t *testing.T) {
	d := eligibility.Evaluate(approvedDriver(), verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now, RequireAvailable: true, MaxLocationAge: time.Minute})
	if !d.Eligible {
		t.Fatalf("expected eligible, got %s", d.Error())
	}
}

func TestHardConstraintsRejectCandidates(t *testing.T) {
	// Document 41's hard-constraint list, each in turn.
	cases := []struct {
		name   string
		mutate func(*eligibility.Driver, *eligibility.Vehicle, *eligibility.Requirements)
		reason eligibility.Reason
	}{
		{"driver not approved", func(d *eligibility.Driver, _ *eligibility.Vehicle, _ *eligibility.Requirements) {
			d.VerificationStatus = "UNDER_REVIEW"
		}, eligibility.ReasonDriverNotApproved},

		{"driver suspended", func(d *eligibility.Driver, _ *eligibility.Vehicle, _ *eligibility.Requirements) {
			d.Status = "SUSPENDED"
		}, eligibility.ReasonDriverUnavailable},

		{"vehicle not verified", func(_ *eligibility.Driver, v *eligibility.Vehicle, _ *eligibility.Requirements) {
			v.VerificationStatus = "PENDING"
		}, eligibility.ReasonVehicleNotVerified},

		{"capability missing", func(_ *eligibility.Driver, _ *eligibility.Vehicle, r *eligibility.Requirements) {
			r.Capability = "HEAVY_CARGO"
		}, eligibility.ReasonCapabilityMissing},

		{"capacity exceeded", func(_ *eligibility.Driver, _ *eligibility.Vehicle, r *eligibility.Requirements) {
			weight := 500.0
			r.WeightKG = &weight
		}, eligibility.ReasonCapacityExceeded},

		{"passengers exceeded", func(_ *eligibility.Driver, _ *eligibility.Vehicle, r *eligibility.Requirements) {
			count := 4
			r.Passengers = &count
		}, eligibility.ReasonPassengersExceeded},

		{"vehicle type excluded", func(_ *eligibility.Driver, _ *eligibility.Vehicle, r *eligibility.Requirements) {
			r.VehicleTypes = []string{"TRUCK", "MAZDA"}
		}, eligibility.ReasonVehicleTypeExcluded},

		{"no active vehicle", func(_ *eligibility.Driver, v *eligibility.Vehicle, _ *eligibility.Requirements) {
			*v = eligibility.Vehicle{}
		}, eligibility.ReasonNoActiveVehicle},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			driver, vehicle := approvedDriver(), verifiedBike()
			req := eligibility.Requirements{Capability: "PARCEL"}
			tc.mutate(&driver, &vehicle, &req)

			d := eligibility.Evaluate(driver, vehicle, req,
				eligibility.Options{Now: now, RequireAvailable: true})
			if d.Eligible {
				t.Fatal("candidate was accepted")
			}
			if !hasReason(d, tc.reason) {
				t.Fatalf("want %s, got %s", tc.reason, d.Error())
			}
		})
	}
}

func TestAnExpiredMandatoryDocumentBlocksWork(t *testing.T) {
	// Document 16: "A driver cannot accept jobs with expired required
	// documents." This is the gate that enforces it.
	expired := now.Add(-24 * time.Hour)
	driver := approvedDriver()
	driver.Documents = []eligibility.Document{
		{Type: "LICENSE", Status: "VERIFIED", ExpiresAt: &expired, Mandatory: true},
	}

	d := eligibility.Evaluate(driver, verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now, RequireAvailable: true})
	if d.Eligible {
		t.Fatal("a driver with an expired licence was allowed to work")
	}
	if !hasReason(d, eligibility.ReasonDocumentExpired) {
		t.Fatalf("want document_expired, got %s", d.Error())
	}
}

func TestADocumentExpiringLaterDoesNotBlock(t *testing.T) {
	future := now.Add(24 * time.Hour)
	driver := approvedDriver()
	driver.Documents = []eligibility.Document{
		{Type: "LICENSE", Status: "VERIFIED", ExpiresAt: &future, Mandatory: true},
	}
	d := eligibility.Evaluate(driver, verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now, RequireAvailable: true})
	if !d.Eligible {
		t.Fatalf("a valid licence blocked work: %s", d.Error())
	}
}

func TestExpiryIsInclusiveAtTheBoundary(t *testing.T) {
	// A document expiring "today" is expired today, not tomorrow. Off by one
	// here means a day of driving on a lapsed permit.
	driver := approvedDriver()
	driver.Documents = []eligibility.Document{
		{Type: "PERMIT", Status: "VERIFIED", ExpiresAt: &now, Mandatory: true},
	}
	d := eligibility.Evaluate(driver, verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now, RequireAvailable: true})
	if d.Eligible {
		t.Fatal("a document expiring at this instant was still accepted")
	}
}

func TestAnUnverifiedDocumentIsNotAValidOne(t *testing.T) {
	driver := approvedDriver()
	driver.Documents = []eligibility.Document{
		{Type: "LICENSE", Status: "PENDING", Mandatory: true},
	}
	d := eligibility.Evaluate(driver, verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now, RequireAvailable: true})
	if !hasReason(d, eligibility.ReasonDocumentMissing) {
		t.Fatalf("want document_missing, got %s", d.Error())
	}
}

func TestOptionalDocumentsDoNotBlock(t *testing.T) {
	expired := now.Add(-time.Hour)
	driver := approvedDriver()
	driver.Documents = []eligibility.Document{
		{Type: "EMERGENCY_CONTACT", Status: "PENDING", ExpiresAt: &expired, Mandatory: false},
	}
	d := eligibility.Evaluate(driver, verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now, RequireAvailable: true})
	if !d.Eligible {
		t.Fatalf("an optional document blocked work: %s", d.Error())
	}
}

func TestUnknownCapacityIsNotUnlimitedCapacity(t *testing.T) {
	// Refusing is the safe reading. Treating unknown as unlimited sends a load
	// to a vehicle that may not carry it.
	vehicle := verifiedBike()
	vehicle.CapacityKG = nil
	weight := 5.0

	d := eligibility.Evaluate(approvedDriver(), vehicle,
		eligibility.Requirements{Capability: "PARCEL", WeightKG: &weight},
		eligibility.Options{Now: now, RequireAvailable: true})
	if d.Eligible {
		t.Fatal("a vehicle with unknown capacity accepted a weighed load")
	}
}

func TestStaleLocationExcludesADriverFromDispatchButNotFromAcceptance(t *testing.T) {
	// Document 16: "Dispatch excludes stale locations." Acceptance is
	// different — the driver is present and answering, so their last position
	// is not the question.
	driver := approvedDriver()
	old := now.Add(-10 * time.Minute)
	driver.LocationAt = &old

	dispatch := eligibility.Evaluate(driver, verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now, RequireAvailable: true, MaxLocationAge: time.Minute})
	if dispatch.Eligible {
		t.Fatal("dispatch considered a driver with a stale location")
	}
	if !hasReason(dispatch, eligibility.ReasonLocationStale) {
		t.Fatalf("want location_stale, got %s", dispatch.Error())
	}

	acceptance := eligibility.Evaluate(driver, verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now})
	if !acceptance.Eligible {
		t.Fatalf("acceptance was blocked by a stale location: %s", acceptance.Error())
	}
}

func TestAnOfferedDriverCanStillAccept(t *testing.T) {
	// A driver answering an offer is OFFERED, not AVAILABLE. Requiring
	// AVAILABLE at acceptance would make every offer impossible to accept.
	driver := approvedDriver()
	driver.Status = eligibility.DriverOffered

	if d := eligibility.Evaluate(driver, verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now}); !d.Eligible {
		t.Fatalf("an offered driver could not accept: %s", d.Error())
	}
	// But dispatch must not offer them a second job.
	if d := eligibility.Evaluate(driver, verifiedBike(),
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{Now: now, RequireAvailable: true}); d.Eligible {
		t.Fatal("dispatch offered a second job to a driver mid-offer")
	}
}

func TestEveryFailureIsReportedNotJustTheFirst(t *testing.T) {
	// A driver fixing their profile should learn all of what is wrong in one
	// pass rather than one problem per attempt.
	driver := approvedDriver()
	driver.VerificationStatus = "REJECTED"
	driver.Status = "BLOCKED"
	vehicle := verifiedBike()
	vehicle.VerificationStatus = "SUSPENDED"

	d := eligibility.Evaluate(driver, vehicle,
		eligibility.Requirements{Capability: "HEAVY_CARGO"},
		eligibility.Options{Now: now, RequireAvailable: true})
	if len(d.Failures) < 4 {
		t.Fatalf("want every failure, got %d: %s", len(d.Failures), d.Error())
	}
	msg := d.Error()
	for _, want := range []string{"driver_not_approved", "driver_unavailable", "vehicle_not_verified", "capability_missing"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%s missing from %q", want, msg)
		}
	}
}

func TestRequirementsFromJobDerivesTheCapability(t *testing.T) {
	cases := map[string]string{
		"RIDE": "PASSENGER", "PARCEL": "PARCEL", "GROCERY": "GROCERY",
		"CARGO": "HEAVY_CARGO", "FREIGHT": "HEAVY_CARGO",
	}
	for jobType, want := range cases {
		if got := eligibility.RequirementsFromJob(jobType, nil).Capability; got != want {
			t.Errorf("%s -> %s, want %s", jobType, got, want)
		}
	}
}

func TestRequirementsFromJobReadsStoredRows(t *testing.T) {
	req := eligibility.RequirementsFromJob("CARGO", map[string]string{
		"capability":    "SMALL_CARGO",
		"weight_kg":     "450.5",
		"passengers":    "2",
		"vehicle_types": "SUZUKI_PICKUP, SHEHZORE",
	})
	if req.Capability != "SMALL_CARGO" {
		t.Errorf("capability = %s", req.Capability)
	}
	if req.WeightKG == nil || *req.WeightKG != 450.5 {
		t.Errorf("weight = %v", req.WeightKG)
	}
	if req.Passengers == nil || *req.Passengers != 2 {
		t.Errorf("passengers = %v", req.Passengers)
	}
	if len(req.VehicleTypes) != 2 || req.VehicleTypes[1] != "SHEHZORE" {
		t.Errorf("vehicle types = %v", req.VehicleTypes)
	}
}

func TestMalformedRequirementRowsAreIgnoredNotGuessed(t *testing.T) {
	req := eligibility.RequirementsFromJob("PARCEL", map[string]string{
		"weight_kg":  "heavy",
		"passengers": "several",
	})
	// A weight the platform cannot parse must not become a weight of zero,
	// which would pass every capacity check.
	if req.WeightKG != nil {
		t.Errorf("an unparseable weight became %v", *req.WeightKG)
	}
	if req.Passengers != nil {
		t.Errorf("an unparseable passenger count became %v", *req.Passengers)
	}
}
