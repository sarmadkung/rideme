//go:build integration

package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/eligibility"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
)

type providerHarness struct {
	store *providers.Store
	pool  *pgxpool.Pool
}

func newProviderHarness(t *testing.T) *providerHarness {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return &providerHarness{store: providers.NewStore(pool), pool: pool}
}

func (h *providerHarness) aUser(t *testing.T) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9231' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *providerHarness) aVehicle(t *testing.T, userID, vehicleType string) providers.Vehicle {
	t.Helper()
	capacity := 750.0
	v, err := h.store.RegisterVehicle(context.Background(), providers.Vehicle{
		OwnerUserID: userID,
		Type:        vehicleType,
		PlateNumber: "LES-" + time.Now().Format("150405.000000"),
		CapacityKG:  &capacity,
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// approvedDriverOn walks the full onboarding funnel — the flow document 29
// draws — rather than writing an approved row directly, so the machine is
// exercised as a real driver would exercise it.
func (h *providerHarness) approvedDriverOn(t *testing.T, vehicleType string) (providers.Driver, providers.Vehicle) {
	t.Helper()
	ctx := context.Background()
	userID := h.aUser(t)

	driver, err := h.store.EnsureDriver(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ from, to providers.VerificationStatus }{
		{providers.VerificationNotStarted, providers.VerificationInProgress},
		{providers.VerificationInProgress, providers.VerificationSubmitted},
		{providers.VerificationSubmitted, providers.VerificationUnderReview},
		{providers.VerificationUnderReview, providers.VerificationApproved},
	} {
		if driver, err = h.store.TransitionVerification(ctx, driver.ID, step.from, step.to, "", ""); err != nil {
			t.Fatalf("%s -> %s: %v", step.from, step.to, err)
		}
	}

	vehicle := h.aVehicle(t, userID, vehicleType)
	if vehicle, err = h.store.TransitionVehicle(ctx, vehicle.ID, providers.VehiclePending, providers.VehicleUnderReview, "", ""); err != nil {
		t.Fatal(err)
	}
	if vehicle, err = h.store.TransitionVehicle(ctx, vehicle.ID, providers.VehicleUnderReview, providers.VehicleVerified, "", ""); err != nil {
		t.Fatal(err)
	}
	if driver, err = h.store.SetActiveVehicle(ctx, driver.ID, vehicle.ID); err != nil {
		t.Fatal(err)
	}
	if driver, err = h.store.TransitionAvailability(ctx, driver.ID, providers.StatusOffline, providers.StatusAvailable); err != nil {
		t.Fatal(err)
	}
	return driver, vehicle
}

func TestOnboardingIsResumableAndIdempotent(t *testing.T) {
	// Document 29: "onboarding is resumable". Tapping "become a driver" twice
	// must not create a second driver or reset progress.
	h := newProviderHarness(t)
	ctx := context.Background()
	userID := h.aUser(t)

	first, err := h.store.EnsureDriver(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.TransitionVerification(ctx, first.ID,
		providers.VerificationNotStarted, providers.VerificationInProgress, "", ""); err != nil {
		t.Fatal(err)
	}

	again, err := h.store.EnsureDriver(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Fatal("a second driver record was created for one user")
	}
	if again.VerificationStatus != providers.VerificationInProgress {
		t.Fatalf("progress was reset to %s", again.VerificationStatus)
	}
}

func TestVerificationAndAvailabilityAreIndependent(t *testing.T) {
	// Document 29's stated objective: verification state must not be mixed
	// with availability state.
	h := newProviderHarness(t)
	ctx := context.Background()
	driver, _ := h.approvedDriverOn(t, "MOTORCYCLE")

	offline, err := h.store.TransitionAvailability(ctx, driver.ID, providers.StatusAvailable, providers.StatusOffline)
	if err != nil {
		t.Fatal(err)
	}
	if offline.VerificationStatus != providers.VerificationApproved {
		t.Fatal("going offline changed verification status")
	}
	if !offline.CanGoOnline() {
		t.Fatal("an approved driver who went offline can no longer go online")
	}
}

func TestCapabilitiesAreDerivedFromTheVehicleTypeNotSubmitted(t *testing.T) {
	// Document 30: "Do not trust capabilities submitted by the client."
	// RegisterVehicle has no capability input at all.
	h := newProviderHarness(t)
	ctx := context.Background()

	bike := h.aVehicle(t, h.aUser(t), "MOTORCYCLE")
	truck := h.aVehicle(t, h.aUser(t), "TRUCK")

	has := func(v providers.Vehicle, code string) bool {
		for _, c := range v.Capabilities {
			if c.Code == code {
				return true
			}
		}
		return false
	}

	if !has(bike, "PASSENGER") || !has(bike, "PARCEL") || !has(bike, "GROCERY") {
		t.Fatalf("motorcycle capabilities = %+v", bike.Capabilities)
	}
	if has(bike, "HEAVY_CARGO") {
		t.Fatal("a motorcycle was given HEAVY_CARGO")
	}
	if !has(truck, "HEAVY_CARGO") || !has(truck, "INTERCITY") {
		t.Fatalf("truck capabilities = %+v", truck.Capabilities)
	}
	if has(truck, "PASSENGER") {
		t.Fatal("a truck was given PASSENGER")
	}
	for _, c := range bike.Capabilities {
		if c.Source != providers.SourceDerived {
			t.Fatalf("capability %s has source %s, want DERIVED", c.Code, c.Source)
		}
	}

	// An admin can grant beyond the type, and the source records that it was a
	// decision rather than a derivation.
	if err := h.store.GrantCapability(ctx, bike.ID, "BUSINESS_DELIVERY"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := h.store.VehicleByID(ctx, bike.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range reloaded.Capabilities {
		if c.Code == "BUSINESS_DELIVERY" && c.Source != providers.SourceAdmin {
			t.Fatalf("granted capability source = %s, want ADMIN", c.Source)
		}
	}
}

func TestTaxonomyIsConfigurationNotSchema(t *testing.T) {
	// Document 30: the taxonomy "must be configuration-friendly because local
	// names/categories can evolve." Adding one must not need a migration.
	h := newProviderHarness(t)
	ctx := context.Background()

	if _, err := h.pool.Exec(ctx,
		`INSERT INTO vehicle_types (code, label, sort_order) VALUES ('TRACTOR_TROLLEY', 'Tractor Trolley', 90)
		 ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO vehicle_type_capabilities (vehicle_type, capability) VALUES ('TRACTOR_TROLLEY', 'HEAVY_CARGO')
		 ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}

	vehicle := h.aVehicle(t, h.aUser(t), "TRACTOR_TROLLEY")
	if len(vehicle.Capabilities) != 1 || vehicle.Capabilities[0].Code != "HEAVY_CARGO" {
		t.Fatalf("a configured vehicle type did not derive its capabilities: %+v", vehicle.Capabilities)
	}

	// And an unknown type is still refused by the foreign key.
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO vehicles (owner_user_id, type, plate_number) VALUES ($1, 'SPACESHIP', 'XYZ-1')`,
		h.aUser(t)); err == nil {
		t.Fatal("an unregistered vehicle type was accepted")
	}
}

func TestOnlyAVerifiedOwnedVehicleCanBecomeActive(t *testing.T) {
	h := newProviderHarness(t)
	ctx := context.Background()
	userID := h.aUser(t)

	driver, err := h.store.EnsureDriver(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	unverified := h.aVehicle(t, userID, "CAR")
	if _, err := h.store.SetActiveVehicle(ctx, driver.ID, unverified.ID); !errors.Is(err, providers.ErrVehicleNotReady) {
		t.Fatalf("an unverified vehicle was activated: %v", err)
	}

	// Someone else's verified vehicle is equally unusable.
	otherDriver, otherVehicle := h.approvedDriverOn(t, "CAR")
	_ = otherDriver
	if _, err := h.store.SetActiveVehicle(ctx, driver.ID, otherVehicle.ID); !errors.Is(err, providers.ErrVehicleNotReady) {
		t.Fatalf("a driver activated another owner's vehicle: %v", err)
	}
}

func TestSuspendingAVehicleClearsItAsAnyonesActiveVehicle(t *testing.T) {
	// Otherwise a driver keeps working on a suspended vehicle until they next
	// change it — which they have no reason to do.
	h := newProviderHarness(t)
	ctx := context.Background()
	driver, vehicle := h.approvedDriverOn(t, "SUZUKI_PICKUP")

	if _, err := h.store.TransitionVehicle(ctx, vehicle.ID,
		providers.VehicleVerified, providers.VehicleSuspended, "", "roadworthiness"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := h.store.DriverByID(ctx, driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ActiveVehicleID != "" {
		t.Fatal("a suspended vehicle is still the driver's active vehicle")
	}
}

func TestReviewActionsAreAudited(t *testing.T) {
	// Document 29: "Every review action is audited."
	h := newProviderHarness(t)
	ctx := context.Background()
	driver, vehicle := h.approvedDriverOn(t, "CAR")

	var driverReviews, vehicleReviews int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM verification_reviews WHERE subject_type = 'DRIVER' AND subject_id = $1`,
		driver.ID).Scan(&driverReviews); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM verification_reviews WHERE subject_type = 'VEHICLE' AND subject_id = $1`,
		vehicle.ID).Scan(&vehicleReviews); err != nil {
		t.Fatal(err)
	}
	if driverReviews != 4 {
		t.Errorf("want 4 driver review rows, got %d", driverReviews)
	}
	if vehicleReviews != 2 {
		t.Errorf("want 2 vehicle review rows, got %d", vehicleReviews)
	}
}

func TestARejectedDriverCanResubmit(t *testing.T) {
	// Document 29's definition of done includes resubmission.
	h := newProviderHarness(t)
	ctx := context.Background()
	driver, err := h.store.EnsureDriver(ctx, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ from, to providers.VerificationStatus }{
		{providers.VerificationNotStarted, providers.VerificationInProgress},
		{providers.VerificationInProgress, providers.VerificationSubmitted},
		{providers.VerificationSubmitted, providers.VerificationRejected},
		{providers.VerificationRejected, providers.VerificationInProgress},
	} {
		if driver, err = h.store.TransitionVerification(ctx, driver.ID, step.from, step.to, "", "blurry photo"); err != nil {
			t.Fatalf("%s -> %s: %v", step.from, step.to, err)
		}
	}
	if driver.VerificationStatus != providers.VerificationInProgress {
		t.Fatalf("status = %s", driver.VerificationStatus)
	}
}

func TestDocumentExpirySweepMarksLapsedCredentials(t *testing.T) {
	h := newProviderHarness(t)
	ctx := context.Background()
	driver, _ := h.approvedDriverOn(t, "CAR")

	yesterday := time.Now().AddDate(0, 0, -1)
	doc, err := h.store.AddDocument(ctx, providers.Document{
		OwnerType: "DRIVER", DriverID: driver.ID, Type: "LICENSE", ExpiresAt: &yesterday,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.ReviewDocument(ctx, doc.ID, providers.DocumentVerified, "", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := h.store.ExpireLapsedDocuments(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := h.pool.QueryRow(ctx, `SELECT status FROM documents WHERE id = $1`, doc.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "EXPIRED" {
		t.Fatalf("status = %s, want EXPIRED", status)
	}
}

func TestAMissingMandatoryDocumentBlocksEligibility(t *testing.T) {
	// The important direction: eligibility must notice a document that was
	// never submitted, not only one that was submitted and is invalid.
	h := newProviderHarness(t)
	ctx := context.Background()
	driver, _ := h.approvedDriverOn(t, "MOTORCYCLE")

	market := "TEST-" + time.Now().Format("150405.000000")
	if err := h.store.SetRequirement(ctx, providers.Requirement{
		Market: market, OwnerType: "DRIVER", Type: "LICENSE", Mandatory: true,
	}); err != nil {
		t.Fatal(err)
	}

	edriver, evehicle, err := h.store.Candidate(ctx, driver.ID, market)
	if err != nil {
		t.Fatal(err)
	}
	decision := eligibility.Evaluate(edriver, evehicle,
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{RequireAvailable: true})
	if decision.Eligible {
		t.Fatal("a driver with no licence on file was eligible")
	}

	// Supplying and verifying it clears the block.
	future := time.Now().AddDate(1, 0, 0)
	doc, err := h.store.AddDocument(ctx, providers.Document{
		OwnerType: "DRIVER", DriverID: driver.ID, Type: "LICENSE", ExpiresAt: &future,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.ReviewDocument(ctx, doc.ID, providers.DocumentVerified, "", ""); err != nil {
		t.Fatal(err)
	}

	edriver, evehicle, err = h.store.Candidate(ctx, driver.ID, market)
	if err != nil {
		t.Fatal(err)
	}
	if d := eligibility.Evaluate(edriver, evehicle,
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{RequireAvailable: true}); !d.Eligible {
		t.Fatalf("a verified licence did not clear the block: %s", d.Error())
	}
}

func TestEligibilityUsesRealProviderState(t *testing.T) {
	// The end-to-end check: the same rules dispatch will use, fed from the
	// database rather than from hand-built structs.
	h := newProviderHarness(t)
	ctx := context.Background()
	driver, _ := h.approvedDriverOn(t, "MOTORCYCLE")

	edriver, evehicle, err := h.store.Candidate(ctx, driver.ID, "PK")
	if err != nil {
		t.Fatal(err)
	}
	if d := eligibility.Evaluate(edriver, evehicle,
		eligibility.Requirements{Capability: "PARCEL"},
		eligibility.Options{RequireAvailable: true}); !d.Eligible {
		t.Fatalf("a fully onboarded driver was ineligible: %s", d.Error())
	}
	// A motorcycle cannot take a cargo haul, whatever the driver's state.
	heavy := 800.0
	if d := eligibility.Evaluate(edriver, evehicle,
		eligibility.Requirements{Capability: "HEAVY_CARGO", WeightKG: &heavy},
		eligibility.Options{RequireAvailable: true}); d.Eligible {
		t.Fatal("a motorcycle was eligible for an 800kg cargo job")
	}
}

func TestConcurrentAvailabilityTransitionsProduceOneWinner(t *testing.T) {
	h := newProviderHarness(t)
	ctx := context.Background()
	driver, _ := h.approvedDriverOn(t, "CAR")

	const racers = 6
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start
			_, err := h.store.TransitionAvailability(ctx, driver.ID,
				providers.StatusAvailable, providers.StatusOffered)
			results <- err
		}()
	}
	close(start)

	var won int
	for i := 0; i < racers; i++ {
		if err := <-results; err == nil {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("%d of %d concurrent transitions succeeded, want exactly 1", won, racers)
	}
}
