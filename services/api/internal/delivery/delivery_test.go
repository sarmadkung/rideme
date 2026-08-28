package delivery_test

import (
	"strings"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/delivery"
)

func f(v float64) *float64 { return &v }

func TestFitsRefusesWhatAVehicleCannotCarry(t *testing.T) {
	// Document 41: "Do not use weight alone." A 3-metre pipe weighing 20kg
	// fits a motorcycle by weight and by nothing else.
	motorcycle := delivery.VehicleCapacity{
		MaxWeightKG: f(30), MaxVolumeM3: f(0.1),
		CargoLengthCM: f(50), CargoWidthCM: f(40), CargoHeightCM: f(40),
	}

	pipe := delivery.Cargo{TotalWeightKG: f(20), LengthCM: f(300)}
	failures := delivery.Fits(pipe, motorcycle, nil)
	if len(failures) == 0 {
		t.Fatal("a 3-metre pipe was accepted on a motorcycle")
	}
	found := false
	for _, failure := range failures {
		if failure == delivery.CapacityLength {
			found = true
		}
	}
	if !found {
		t.Fatalf("length was not the reported failure: %v", failures)
	}
}

func TestFitsAcceptsALoadWithinEveryLimit(t *testing.T) {
	truck := delivery.VehicleCapacity{
		MaxWeightKG: f(5000), MaxVolumeM3: f(20),
		CargoLengthCM: f(600), CargoWidthCM: f(240), CargoHeightCM: f(240),
	}
	load := delivery.Cargo{
		TotalWeightKG: f(1200), VolumeM3: f(8),
		LengthCM: f(400), WidthCM: f(200), HeightCM: f(180),
	}
	if failures := delivery.Fits(load, truck, nil); len(failures) != 0 {
		t.Fatalf("a load within every limit was refused: %v", failures)
	}
}

func TestUnknownVehicleCapacityFails(t *testing.T) {
	// Passing would dispatch a load to a vehicle that may not carry it, which
	// is discovered at the pickup by a driver who cannot do the job.
	unknown := delivery.VehicleCapacity{}
	load := delivery.Cargo{TotalWeightKG: f(500)}

	failures := delivery.Fits(load, unknown, nil)
	if len(failures) == 0 {
		t.Fatal("a vehicle with unknown capacity accepted a weighed load")
	}
	if failures[0] != delivery.CapacityUnknown {
		t.Fatalf("failure = %v, want vehicle_capacity_unknown", failures[0])
	}
}

func TestACargoWithNoStatedDimensionsIsNotConstrainedByThem(t *testing.T) {
	// An unmeasured dimension is not a violated one; only what the customer
	// declared is checked.
	vehicle := delivery.VehicleCapacity{MaxWeightKG: f(1000)}
	load := delivery.Cargo{TotalWeightKG: f(500)}
	if failures := delivery.Fits(load, vehicle, nil); len(failures) != 0 {
		t.Fatalf("undeclared dimensions caused failures: %v", failures)
	}
}

func TestMissingEquipmentIsAHardConstraint(t *testing.T) {
	// Document 80 lists required_equipment among the hard constraints.
	vehicle := delivery.VehicleCapacity{MaxWeightKG: f(2000), Equipment: []string{"STRAPS"}}
	load := delivery.Cargo{TotalWeightKG: f(500)}

	if failures := delivery.Fits(load, vehicle, []string{"STRAPS"}); len(failures) != 0 {
		t.Fatalf("equipment the vehicle has was reported missing: %v", failures)
	}
	failures := delivery.Fits(load, vehicle, []string{"TAIL_LIFT"})
	if len(failures) == 0 || failures[0] != delivery.CapacityEquipment {
		t.Fatalf("missing equipment was not a failure: %v", failures)
	}
}

func TestFailureActionsAreDeterministic(t *testing.T) {
	// Document 84 requires a deterministic next action. A parcel with no
	// defined next step is a parcel nobody owns.
	policy := delivery.RetryPolicy{MaxAttempts: 3}

	cases := []struct {
		reason  delivery.FailureReason
		attempt int
		want    delivery.NextAction
	}{
		// Retrying the same wrong address produces the same failure.
		{delivery.FailureWrongAddress, 1, delivery.ActionEscalate},
		// The recipient said no; retrying is harassment.
		{delivery.FailureRecipientRejected, 1, delivery.ActionReturn},
		// Neither is the driver's to resolve on the doorstep.
		{delivery.FailureDamagedPackage, 1, delivery.ActionEscalate},
		{delivery.FailureMerchantIssue, 1, delivery.ActionEscalate},
		// These are worth another go, until the limit.
		{delivery.FailureRecipientUnavailable, 1, delivery.ActionRetry},
		{delivery.FailureRecipientUnavailable, 2, delivery.ActionRetry},
		{delivery.FailureRecipientUnavailable, 3, delivery.ActionReturn},
		{delivery.FailureAccessBlocked, 1, delivery.ActionRetry},
		{delivery.FailureAccessBlocked, 3, delivery.ActionReturn},
	}
	for _, tc := range cases {
		if got := delivery.DecideNextAction(tc.reason, tc.attempt, policy); got != tc.want {
			t.Errorf("%s attempt %d -> %s, want %s", tc.reason, tc.attempt, got, tc.want)
		}
	}
}

func TestRetriesAreBounded(t *testing.T) {
	policy := delivery.RetryPolicy{MaxAttempts: 2}
	if action := delivery.DecideNextAction(delivery.FailureRecipientUnavailable, 5, policy); action == delivery.ActionRetry {
		t.Fatal("retries were unbounded")
	}
}

func TestCustomerMessagesExposeNoInternalCodes(t *testing.T) {
	// Document 84: "avoid exposing internal failure codes". A customer told
	// MERCHANT_ISSUE learns nothing and is alarmed by it.
	for _, reason := range []delivery.FailureReason{
		delivery.FailureRecipientUnavailable, delivery.FailureWrongAddress,
		delivery.FailureRecipientRejected, delivery.FailureDamagedPackage,
		delivery.FailureAccessBlocked, delivery.FailureMerchantIssue,
	} {
		message := delivery.CustomerMessage(reason)
		if message == "" {
			t.Errorf("%s has no customer message", reason)
		}
		if strings.Contains(message, string(reason)) || strings.Contains(message, "_") {
			t.Errorf("%s leaked its internal code: %q", reason, message)
		}
	}
}

func TestChargeableWaitingRespectsTheGracePeriod(t *testing.T) {
	// Document 87: waiting becomes chargeable only after a configurable grace.
	arrived := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	// Within grace: nothing charged.
	if got := delivery.ChargeableWaiting(arrived, arrived.Add(4*time.Minute), 300); got != 0 {
		t.Errorf("charged %ds within a 5-minute grace", got)
	}
	// Exactly at grace: still nothing.
	if got := delivery.ChargeableWaiting(arrived, arrived.Add(5*time.Minute), 300); got != 0 {
		t.Errorf("charged %ds at exactly the grace boundary", got)
	}
	// Past grace: only the excess.
	if got := delivery.ChargeableWaiting(arrived, arrived.Add(20*time.Minute), 300); got != 900 {
		t.Errorf("charged %ds, want 900 (20 minutes less 5 grace)", got)
	}
}

func TestChargeableWaitingIgnoresNonsense(t *testing.T) {
	arrived := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	// A departure before the arrival is a clock problem, not negative waiting.
	if got := delivery.ChargeableWaiting(arrived, arrived.Add(-time.Hour), 0); got != 0 {
		t.Errorf("got %d for a negative interval", got)
	}
	if got := delivery.ChargeableWaiting(time.Time{}, arrived, 0); got != 0 {
		t.Errorf("got %d with no arrival time", got)
	}
}

func TestEachServiceHasARequiredProofMethod(t *testing.T) {
	// Document 83: "Every completed delivery has a policy-compliant proof
	// record." A ride needs none — the passenger got out.
	if _, required := delivery.RequiredProof("RIDE"); required {
		t.Error("a ride was made to require proof of delivery")
	}
	for _, jobType := range []string{"PARCEL", "GROCERY", "CARGO", "FREIGHT"} {
		method, required := delivery.RequiredProof(jobType)
		if !required {
			t.Errorf("%s requires no proof", jobType)
		}
		if !method.Valid() {
			t.Errorf("%s requires an unknown method %q", jobType, method)
		}
	}
}

func TestProofValidationDemandsEvidenceWhereTheMethodImpliesIt(t *testing.T) {
	// A photo proof with no photo is not proof.
	if err := delivery.ValidateProof(delivery.Proof{Method: delivery.ProofPhoto}); err == nil {
		t.Error("a photo proof with no media reference was accepted")
	}
	if err := delivery.ValidateProof(delivery.Proof{Method: delivery.ProofSignature}); err == nil {
		t.Error("a signature proof with no media reference was accepted")
	}
	if err := delivery.ValidateProof(delivery.Proof{
		Method: delivery.ProofPhoto, MediaKey: "s3://proofs/abc",
	}); err != nil {
		t.Errorf("a complete photo proof was refused: %v", err)
	}
	// An OTP proof carries no media by nature.
	if err := delivery.ValidateProof(delivery.Proof{Method: delivery.ProofRecipientOTP}); err != nil {
		t.Errorf("an OTP proof was refused for having no media: %v", err)
	}
	if err := delivery.ValidateProof(delivery.Proof{Method: "TELEPATHY"}); err == nil {
		t.Error("an unknown proof method was accepted")
	}
}
