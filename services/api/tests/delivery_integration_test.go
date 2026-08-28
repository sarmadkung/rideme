//go:build integration

package tests

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/delivery"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
)

type deliveryHarness struct {
	store *delivery.Store
	jobs  *jobs.Store
	pool  *pgxpool.Pool
}

func newDeliveryHarness(t *testing.T) *deliveryHarness {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return &deliveryHarness{
		store: delivery.NewStore(pool, "delivery-test-secret-at-least-32-bytes"),
		jobs:  jobs.NewStore(pool),
		pool:  pool,
	}
}

func (h *deliveryHarness) aParcelJob(t *testing.T) jobs.Job {
	t.Helper()
	var userID string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9239' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	job, err := h.jobs.Create(context.Background(), jobs.Job{
		Type:            jobs.TypeParcel,
		RequesterUserID: userID,
		Status:          jobs.StatusInProgress,
		Stops: []jobs.Stop{
			{Sequence: 0, Type: jobs.StopPickup, Location: jobs.Coordinate{Latitude: 31.5204, Longitude: 74.3587}, Address: "sender"},
			{Sequence: 1, Type: jobs.StopDropoff, Location: jobs.Coordinate{Latitude: 31.5880, Longitude: 74.3150}, Address: "recipient"},
		},
	}, jobs.Actor{Type: jobs.ActorCustomer})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestRecipientOTPIsHashedAndSingleUse(t *testing.T) {
	// Document 83: "Never expose the raw OTP to unauthorized parties." This
	// code authorises a handover, so a stored code is a stolen parcel.
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)
	dropoff, _ := job.Dropoff()

	code, err := h.store.IssueRecipientOTP(ctx, job.ID, dropoff.ID)
	if err != nil {
		t.Fatal(err)
	}

	var stored []byte
	if err := h.pool.QueryRow(ctx,
		`SELECT code_hash FROM delivery_otps WHERE stop_id = $1`, dropoff.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if string(stored) == code || len(stored) != 32 {
		t.Fatal("the code is not stored as a 32-byte hash")
	}

	if err := h.store.VerifyRecipientOTP(ctx, dropoff.ID, code); err != nil {
		t.Fatalf("the correct code failed: %v", err)
	}
	// Replaying it would authorise a second handover of a parcel already gone.
	if err := h.store.VerifyRecipientOTP(ctx, dropoff.ID, code); !errors.Is(err, delivery.ErrOTPIncorrect) {
		t.Fatalf("a consumed code was accepted again: %v", err)
	}
}

func TestConcurrentOTPVerificationsAdmitOneHandover(t *testing.T) {
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)
	dropoff, _ := job.Dropoff()

	code, err := h.store.IssueRecipientOTP(ctx, job.ID, dropoff.ID)
	if err != nil {
		t.Fatal(err)
	}

	const racers = 8
	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- h.store.VerifyRecipientOTP(ctx, dropoff.ID, code)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var accepted int
	for err := range results {
		if err == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("%d of %d concurrent verifications succeeded, want 1", accepted, racers)
	}
}

func TestAWrongRecipientCodeIsRejectedAndBounded(t *testing.T) {
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)
	dropoff, _ := job.Dropoff()

	code, err := h.store.IssueRecipientOTP(ctx, job.ID, dropoff.ID)
	if err != nil {
		t.Fatal(err)
	}
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}

	for i := 0; i < 5; i++ {
		if err := h.store.VerifyRecipientOTP(ctx, dropoff.ID, wrong); !errors.Is(err, delivery.ErrOTPIncorrect) {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	// Past the limit even the right code fails, or the limit only slows an
	// attacker rather than stopping them.
	if err := h.store.VerifyRecipientOTP(ctx, dropoff.ID, code); !errors.Is(err, delivery.ErrOTPIncorrect) {
		t.Fatalf("the correct code worked after the attempt limit: %v", err)
	}
}

func TestReissuingACodeInvalidatesTheOldOne(t *testing.T) {
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)
	dropoff, _ := job.Dropoff()

	first, err := h.store.IssueRecipientOTP(ctx, job.ID, dropoff.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.store.IssueRecipientOTP(ctx, job.ID, dropoff.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Skip("the generator produced the same code twice; nothing to assert")
	}
	if err := h.store.VerifyRecipientOTP(ctx, dropoff.ID, first); err == nil {
		t.Fatal("a superseded code still worked")
	}
	if err := h.store.VerifyRecipientOTP(ctx, dropoff.ID, second); err != nil {
		t.Fatalf("the current code failed: %v", err)
	}
}

func TestProofIsRecordedWithItsAuditFields(t *testing.T) {
	// Document 83's audit list: method, timestamp, actor, location, media
	// reference, verification result.
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)
	dropoff, _ := job.Dropoff()

	proof, err := h.store.RecordProof(ctx, delivery.Proof{
		JobID: job.ID, StopID: dropoff.ID, Method: delivery.ProofPhoto,
		MediaKey: "s3://proofs/abc.jpg", RecipientName: "recipient",
		Verified: true, Lat: 31.5880, Lon: 74.3150, HasLocation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if proof.ID == "" || proof.CreatedAt.IsZero() {
		t.Fatalf("proof not stored: %+v", proof)
	}

	// The binary is not in the job database — only a reference (document 83).
	var mediaKey string
	var hasLocation bool
	if err := h.pool.QueryRow(ctx,
		`SELECT media_key, location IS NOT NULL FROM delivery_proofs WHERE id = $1`,
		proof.ID).Scan(&mediaKey, &hasLocation); err != nil {
		t.Fatal(err)
	}
	if mediaKey != "s3://proofs/abc.jpg" {
		t.Fatalf("media reference = %q", mediaKey)
	}
	if !hasLocation {
		t.Fatal("the capture location was not recorded")
	}

	proofs, err := h.store.ProofsFor(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(proofs) != 1 {
		t.Fatalf("%d proofs for the job", len(proofs))
	}
}

func TestAFailedDeliveryProducesADeterministicNextAction(t *testing.T) {
	// Document 84: failures must have deterministic next actions and remain
	// fully traceable.
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)
	dropoff, _ := job.Dropoff()
	policy := delivery.DefaultRetryPolicy()

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		action := delivery.DecideNextAction(delivery.FailureRecipientUnavailable, attempt, policy)
		if _, err := h.store.RecordAttempt(ctx, delivery.Attempt{
			JobID: job.ID, StopID: dropoff.ID, Attempt: attempt,
			Reason: delivery.FailureRecipientUnavailable, NextAction: action,
		}); err != nil {
			t.Fatal(err)
		}
	}

	count, err := h.store.AttemptCount(ctx, dropoff.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != policy.MaxAttempts {
		t.Fatalf("%d attempts recorded, want %d", count, policy.MaxAttempts)
	}

	// The final attempt returns rather than retrying forever.
	var lastAction string
	if err := h.pool.QueryRow(ctx,
		`SELECT next_action FROM delivery_attempts WHERE stop_id = $1 ORDER BY attempt DESC LIMIT 1`,
		dropoff.ID).Scan(&lastAction); err != nil {
		t.Fatal(err)
	}
	if lastAction != string(delivery.ActionReturn) {
		t.Fatalf("last action = %s, want RETURN", lastAction)
	}
}

func TestAReturnAddsAStopRatherThanANewJob(t *testing.T) {
	// Document 84: a return "may create a Return Stop rather than creating an
	// unrelated manual job". One job keeps the parcel's history in one place.
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)
	dropoff, _ := job.Dropoff()
	pickup, _ := job.Pickup()

	returnStopID, err := h.store.AddReturnStop(ctx, job.ID, dropoff.ID)
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := h.jobs.ByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Stops) != 3 {
		t.Fatalf("%d stops after the return, want 3", len(reloaded.Stops))
	}

	// The return goes back to where the parcel came from.
	var isReturn bool
	var lat, lon float64
	if err := h.pool.QueryRow(ctx,
		`SELECT is_return, ST_Y(location::geometry), ST_X(location::geometry)
		   FROM job_stops WHERE id = $1`, returnStopID).Scan(&isReturn, &lat, &lon); err != nil {
		t.Fatal(err)
	}
	if !isReturn {
		t.Error("the return stop is not marked as one")
	}
	if d := lat - pickup.Location.Latitude; d > 0.0001 || d < -0.0001 {
		t.Errorf("the return goes to %v, not back to the pickup at %v", lat, pickup.Location.Latitude)
	}

	// No second job was created.
	var jobCount int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM jobs WHERE requester_user_id = $1`, job.RequesterUserID).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 {
		t.Fatalf("%d jobs exist; a return created a separate job", jobCount)
	}
}

func TestCargoCapacityIsCheckedAgainstRealVehicles(t *testing.T) {
	// Document 80's definition of done: "Dispatch cannot assign cargo to a
	// vehicle that is physically or operationally incapable of carrying it."
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)

	weight, length := 800.0, 350.0
	if err := h.store.SaveCargo(ctx, delivery.Cargo{
		JobID: job.ID, TotalWeightKG: &weight, LengthCM: &length,
		LoadingAssistance: delivery.AssistDriverPlusHelper,
	}); err != nil {
		t.Fatal(err)
	}

	cargo, found, err := h.store.CargoFor(ctx, job.ID)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if cargo.LoadingAssistance != delivery.AssistDriverPlusHelper {
		t.Fatalf("assistance = %s", cargo.LoadingAssistance)
	}

	var ownerID, bikeID, truckID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO users (phone) VALUES ('+9240' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO vehicles (owner_user_id, type, plate_number, capacity_kg, cargo_length_cm)
		 VALUES ($1, 'MOTORCYCLE', 'BK-' || floor(random()*100000000)::text, 30, 50)
		 RETURNING id::text`, ownerID).Scan(&bikeID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO vehicles (owner_user_id, type, plate_number, capacity_kg, cargo_length_cm, equipment)
		 VALUES ($1, 'TRUCK', 'TR-' || floor(random()*100000000)::text, 5000, 600, ARRAY['STRAPS'])
		 RETURNING id::text`, ownerID).Scan(&truckID); err != nil {
		t.Fatal(err)
	}

	bike, err := h.store.VehicleCapacityFor(ctx, bikeID)
	if err != nil {
		t.Fatal(err)
	}
	if failures := delivery.Fits(cargo, bike, nil); len(failures) == 0 {
		t.Fatal("an 800kg, 3.5m load was accepted on a motorcycle")
	}

	truck, err := h.store.VehicleCapacityFor(ctx, truckID)
	if err != nil {
		t.Fatal(err)
	}
	if failures := delivery.Fits(cargo, truck, nil); len(failures) != 0 {
		t.Fatalf("a truck was refused a load it can carry: %v", failures)
	}
	// Equipment is a hard constraint too.
	if failures := delivery.Fits(cargo, truck, []string{"TAIL_LIFT"}); len(failures) == 0 {
		t.Fatal("a truck without a tail lift accepted a load requiring one")
	}
}

func TestWaitingAndLoadingTimesAreRecorded(t *testing.T) {
	// Document 87: cargo time must be captured "without corrupting the main
	// job state". BD-13 leaves the rates open; the record does not depend on
	// them.
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)
	pickup, _ := job.Pickup()

	if err := h.store.MarkArrived(ctx, job.ID, pickup.ID, 300); err != nil {
		t.Fatal(err)
	}
	if err := h.store.MarkLoadingStarted(ctx, pickup.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.store.MarkLoaded(ctx, pickup.ID); err != nil {
		t.Fatal(err)
	}

	timing, found, err := h.store.TimingFor(ctx, pickup.ID)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if timing.ArrivedAt == nil || timing.LoadingStartedAt == nil || timing.LoadedAt == nil {
		t.Fatalf("timestamps missing: %+v", timing)
	}
	// Within the grace period nothing is chargeable, and no money appears
	// anywhere in this record.
	if timing.ChargeableWaitingSecs != 0 {
		t.Errorf("charged %ds of waiting inside a 5-minute grace", timing.ChargeableWaitingSecs)
	}
	if timing.GraceSeconds != 300 {
		t.Errorf("grace = %d", timing.GraceSeconds)
	}
}

func TestArrivalIsIdempotent(t *testing.T) {
	// A driver tapping "arrived" twice must not restart the waiting clock and
	// erase the time already accrued.
	h := newDeliveryHarness(t)
	ctx := context.Background()
	job := h.aParcelJob(t)
	pickup, _ := job.Pickup()

	if err := h.store.MarkArrived(ctx, job.ID, pickup.ID, 0); err != nil {
		t.Fatal(err)
	}
	first, _, err := h.store.TimingFor(ctx, pickup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.MarkArrived(ctx, job.ID, pickup.ID, 0); err != nil {
		t.Fatal(err)
	}
	again, _, err := h.store.TimingFor(ctx, pickup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !first.ArrivedAt.Equal(*again.ArrivedAt) {
		t.Fatal("a repeated arrival reset the clock")
	}
}

func TestRestrictedGoodsPassVacuouslyUntilAListExists(t *testing.T) {
	// BD-13 and document 88: the list is legal and the owner's to supply. A
	// guessed list would block legitimate loads and miss real ones, so the
	// table ships empty and the check is honest about it.
	h := newDeliveryHarness(t)
	ctx := context.Background()

	blocked, err := h.store.CheckRestricted(ctx, "PK", []string{"EXPLOSIVES", "LIVESTOCK"})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 0 {
		t.Fatalf("the empty list blocked %v; nothing should be restricted yet", blocked)
	}

	// Once a list exists, the mechanism works.
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO restricted_goods (code, label) VALUES ('EXPLOSIVES', 'Explosives')
		 ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	blocked, err = h.store.CheckRestricted(ctx, "PK", []string{"EXPLOSIVES", "LIVESTOCK"})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || blocked[0] != "EXPLOSIVES" {
		t.Fatalf("blocked = %v, want [EXPLOSIVES]", blocked)
	}
	if _, err := h.pool.Exec(ctx, `DELETE FROM restricted_goods WHERE code = 'EXPLOSIVES'`); err != nil {
		t.Fatal(err)
	}
}
