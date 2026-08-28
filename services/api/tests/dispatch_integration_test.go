//go:build integration

package tests

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/dispatch"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
)

// Document 46 asks for exactly this: "Write concurrency tests that start many
// assignment attempts simultaneously. Expected: 1 winner, N losers, 0 corrupted
// jobs, 0 double assignments." These are that test, run against real Postgres,
// because the guarantees are database constraints and conditional writes —
// a mock would assert nothing about either.

type dispatchHarness struct {
	store *dispatch.Store
	jobs  *jobs.Store
	pool  *pgxpool.Pool
}

func newDispatchHarness(t *testing.T) *dispatchHarness {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return &dispatchHarness{store: dispatch.NewStore(pool), jobs: jobs.NewStore(pool), pool: pool}
}

func (h *dispatchHarness) aUser(t *testing.T) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9238' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// anApprovedDriver creates a driver who would pass the in-transaction
// eligibility re-check inside Accept.
func (h *dispatchHarness) anApprovedDriver(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	userID := h.aUser(t)

	var driverID, vehicleID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO drivers (user_id, verification_status, status)
		 VALUES ($1, 'APPROVED', 'AVAILABLE') RETURNING id::text`, userID).Scan(&driverID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO vehicles (owner_user_id, type, plate_number, verification_status)
		 VALUES ($1, 'CAR', 'DSP-' || floor(random()*100000000)::text, 'VERIFIED')
		 RETURNING id::text`, userID).Scan(&vehicleID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx,
		`UPDATE drivers SET active_vehicle_id = $2 WHERE id = $1`, driverID, vehicleID); err != nil {
		t.Fatal(err)
	}
	return driverID
}

func (h *dispatchHarness) aSearchingJob(t *testing.T) jobs.Job {
	t.Helper()
	job, err := h.jobs.Create(context.Background(), jobs.Job{
		Type:            jobs.TypeRide,
		RequesterUserID: h.aUser(t),
		Status:          jobs.StatusSearching,
		Stops: []jobs.Stop{
			{Sequence: 0, Type: jobs.StopPickup, Location: jobs.Coordinate{Latitude: 31.5204, Longitude: 74.3587}},
			{Sequence: 1, Type: jobs.StopDropoff, Location: jobs.Coordinate{Latitude: 31.5880, Longitude: 74.3150}},
		},
	}, jobs.Actor{Type: jobs.ActorCustomer})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func (h *dispatchHarness) offer(t *testing.T, jobID, driverID string, ttl time.Duration) {
	t.Helper()
	if _, _, err := h.store.Reserve(context.Background(), jobID, driverID, "", "", 0.9, ttl); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := h.jobs.Transition(context.Background(), jobID, jobs.StatusSearching, jobs.StatusAssigned,
		jobs.Actor{Type: jobs.ActorSystem}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestOneJobOfferedToManyDriversYieldsOneReservation(t *testing.T) {
	// Document 46's main race, at the reservation step.
	h := newDispatchHarness(t)
	job := h.aSearchingJob(t)

	const racers = 12
	drivers := make([]string, racers)
	for i := range drivers {
		drivers[i] = h.anApprovedDriver(t)
	}

	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for _, driverID := range drivers {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			_, _, err := h.store.Reserve(context.Background(), job.ID, id, "", "", 0.5, time.Minute)
			results <- err
		}(driverID)
	}
	close(start)
	wg.Wait()
	close(results)

	var won int
	for err := range results {
		switch {
		case err == nil:
			won++
		case errors.Is(err, dispatch.ErrJobClaimed), errors.Is(err, dispatch.ErrDriverBusy):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d of %d reservations succeeded, want exactly 1", won, racers)
	}

	var live int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM assignments WHERE job_id = $1 AND status IN ('OFFERED','ACCEPTED')`,
		job.ID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 1 {
		t.Fatalf("%d live assignments exist for one job", live)
	}
}

func TestOneDriverCannotHoldTwoReservations(t *testing.T) {
	// Document 46: "At most one active job/reservation may consume a driver's
	// dispatch capacity." Without this the driver accepts one job and the
	// other is stranded holding a reservation nobody will consume.
	h := newDispatchHarness(t)
	driverID := h.anApprovedDriver(t)

	const racers = 8
	jobIDs := make([]string, racers)
	for i := range jobIDs {
		jobIDs[i] = h.aSearchingJob(t).ID
	}

	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for _, jobID := range jobIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			_, _, err := h.store.Reserve(context.Background(), id, driverID, "", "", 0.5, time.Minute)
			results <- err
		}(jobID)
	}
	close(start)
	wg.Wait()
	close(results)

	var won int
	for err := range results {
		if err == nil {
			won++
		} else if !errors.Is(err, dispatch.ErrDriverBusy) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("one driver held %d reservations, want 1", won)
	}
}

func TestConcurrentAcceptancesYieldOneWinner(t *testing.T) {
	// The acceptance race: a driver's client retrying, or two gateway
	// instances delivering the same tap.
	h := newDispatchHarness(t)
	job := h.aSearchingJob(t)
	driverID := h.anApprovedDriver(t)
	h.offer(t, job.ID, driverID, time.Minute)

	const racers = 10
	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := h.store.Accept(context.Background(), job.ID, driverID)
			results <- err
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
		t.Fatalf("%d of %d acceptances succeeded, want exactly 1", accepted, racers)
	}

	// 0 corrupted jobs: exactly one accepted assignment, and the job holds the
	// driver who won.
	var acceptedAssignments int
	var assignedDriver *string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM assignments WHERE job_id = $1 AND status = 'ACCEPTED'`,
		job.ID).Scan(&acceptedAssignments); err != nil {
		t.Fatal(err)
	}
	if acceptedAssignments != 1 {
		t.Fatalf("%d accepted assignments", acceptedAssignments)
	}
	if err := h.pool.QueryRow(context.Background(),
		`SELECT assigned_driver_id::text FROM jobs WHERE id = $1`, job.ID).Scan(&assignedDriver); err != nil {
		t.Fatal(err)
	}
	if assignedDriver == nil || *assignedDriver != driverID {
		t.Fatalf("job assigned to %v, want %s", assignedDriver, driverID)
	}
}

func TestTheDatabaseRefusesASecondLiveOfferForOneJob(t *testing.T) {
	// The invariant beneath everything else in this file. Two drivers holding
	// one job does not produce a wrong number — it sends two people to the
	// same customer and pays for both.
	//
	// This is asserted against raw SQL rather than through the store, because
	// the guarantee must hold even for code that bypasses the application: a
	// migration, an operator script, a module written later by someone who did
	// not read this file.
	h := newDispatchHarness(t)
	ctx := context.Background()
	job := h.aSearchingJob(t)
	first := h.anApprovedDriver(t)
	second := h.anApprovedDriver(t)

	h.offer(t, job.ID, first, time.Minute)

	_, err := h.pool.Exec(ctx,
		`INSERT INTO assignments (job_id, driver_id, status, expires_at)
		 VALUES ($1, $2, 'OFFERED', now() + interval '1 minute')`, job.ID, second)
	if err == nil {
		t.Fatal("the database accepted a second live offer for one job")
	}

	// An ACCEPTED assignment is equally exclusive.
	if _, err := h.store.Accept(ctx, job.ID, first); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO assignments (job_id, driver_id, status)
		 VALUES ($1, $2, 'OFFERED')`, job.ID, second); err == nil {
		t.Fatal("the database offered a job that another driver had already accepted")
	}

	// Terminal assignments do not block: a rejected offer is history, and the
	// job must remain offerable.
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO assignments (job_id, driver_id, status)
		 VALUES ($1, $2, 'REJECTED')`, job.ID, second); err != nil {
		t.Fatalf("a historical rejected assignment was refused: %v", err)
	}
}

func TestAnExpiredOfferCannotBeAccepted(t *testing.T) {
	// Document 43: "expired offers cannot be accepted".
	h := newDispatchHarness(t)
	job := h.aSearchingJob(t)
	driverID := h.anApprovedDriver(t)
	h.offer(t, job.ID, driverID, time.Minute)

	if _, err := h.pool.Exec(context.Background(),
		`UPDATE assignments SET expires_at = now() - interval '1 second'
		  WHERE job_id = $1 AND driver_id = $2`, job.ID, driverID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Accept(context.Background(), job.ID, driverID); !errors.Is(err, dispatch.ErrOfferNotFound) {
		t.Fatalf("an expired offer was accepted: %v", err)
	}
}

func TestADriverSuspendedAfterTheOfferCannotAccept(t *testing.T) {
	// Document 43: "The driver must still pass an authoritative availability
	// check." The re-check happens inside the acceptance transaction.
	h := newDispatchHarness(t)
	job := h.aSearchingJob(t)
	driverID := h.anApprovedDriver(t)
	h.offer(t, job.ID, driverID, time.Minute)

	if _, err := h.pool.Exec(context.Background(),
		`UPDATE drivers SET verification_status = 'SUSPENDED' WHERE id = $1`, driverID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Accept(context.Background(), job.ID, driverID); !errors.Is(err, dispatch.ErrNotEligible) {
		t.Fatalf("a suspended driver accepted a job: %v", err)
	}

	// And the job is not left assigned to them.
	var assigned *string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT assigned_driver_id::text FROM jobs WHERE id = $1`, job.ID).Scan(&assigned); err != nil {
		t.Fatal(err)
	}
	if assigned != nil {
		t.Fatalf("the job was assigned to a suspended driver: %v", *assigned)
	}
}

func TestRejectionReturnsTheJobAndFreesTheDriver(t *testing.T) {
	// Document 45: rejection releases the assignment and returns the job to
	// SEARCHING, without overwriting assignment history.
	h := newDispatchHarness(t)
	job := h.aSearchingJob(t)
	driverID := h.anApprovedDriver(t)
	h.offer(t, job.ID, driverID, time.Minute)

	if _, err := h.pool.Exec(context.Background(),
		`UPDATE drivers SET status = 'OFFERED' WHERE id = $1`, driverID); err != nil {
		t.Fatal(err)
	}
	if err := h.store.Reject(context.Background(), job.ID, driverID); err != nil {
		t.Fatal(err)
	}

	var jobStatus, driverStatus, assignmentStatus string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT status FROM jobs WHERE id = $1`, job.ID).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(context.Background(),
		`SELECT status FROM drivers WHERE id = $1`, driverID).Scan(&driverStatus); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(context.Background(),
		`SELECT status FROM assignments WHERE job_id = $1 AND driver_id = $2`,
		job.ID, driverID).Scan(&assignmentStatus); err != nil {
		t.Fatal(err)
	}

	if jobStatus != "SEARCHING" {
		t.Errorf("job is %s, want SEARCHING", jobStatus)
	}
	if driverStatus != "AVAILABLE" {
		t.Errorf("driver is %s, want AVAILABLE — a declined offer must not strand them", driverStatus)
	}
	// History is preserved: the rejection is visible, not erased.
	if assignmentStatus != "REJECTED" {
		t.Errorf("assignment is %s, want REJECTED", assignmentStatus)
	}

	// And the job can be offered to someone else.
	if _, _, err := h.store.Reserve(context.Background(), job.ID, h.anApprovedDriver(t), "", "", 0.5, time.Minute); err != nil {
		t.Fatalf("the job stayed locked after a rejection: %v", err)
	}
}

func TestTheExpirySweepReleasesAbandonedOffers(t *testing.T) {
	// A timeout that lives in one process stops working when that process
	// restarts. The sweep is the durable backstop (document 43's timeout path).
	h := newDispatchHarness(t)
	job := h.aSearchingJob(t)
	driverID := h.anApprovedDriver(t)
	h.offer(t, job.ID, driverID, time.Minute)
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE drivers SET status = 'OFFERED' WHERE id = $1`, driverID); err != nil {
		t.Fatal(err)
	}

	if _, err := h.store.SweepExpired(context.Background(), time.Now().Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	var assignmentStatus, reservationState, jobStatus, driverStatus string
	q := h.pool.QueryRow
	if err := q(context.Background(), `SELECT status FROM assignments WHERE job_id = $1`, job.ID).Scan(&assignmentStatus); err != nil {
		t.Fatal(err)
	}
	if err := q(context.Background(), `SELECT state FROM driver_reservations WHERE job_id = $1`, job.ID).Scan(&reservationState); err != nil {
		t.Fatal(err)
	}
	if err := q(context.Background(), `SELECT status FROM jobs WHERE id = $1`, job.ID).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if err := q(context.Background(), `SELECT status FROM drivers WHERE id = $1`, driverID).Scan(&driverStatus); err != nil {
		t.Fatal(err)
	}

	// Document 43's exact sequence: offer → EXPIRED, reservation → RELEASED,
	// job → SEARCHING.
	if assignmentStatus != "EXPIRED" {
		t.Errorf("assignment is %s, want EXPIRED", assignmentStatus)
	}
	if reservationState != "EXPIRED" {
		t.Errorf("reservation is %s, want EXPIRED", reservationState)
	}
	if jobStatus != "SEARCHING" {
		t.Errorf("job is %s, want SEARCHING", jobStatus)
	}
	if driverStatus != "AVAILABLE" {
		t.Errorf("driver is %s, want AVAILABLE", driverStatus)
	}

	// After release the driver can be reserved again — the whole point.
	if _, _, err := h.store.Reserve(context.Background(), h.aSearchingJob(t).ID, driverID, "", "", 0.5, time.Minute); err != nil {
		t.Fatalf("the driver was still held after expiry: %v", err)
	}
}

func TestDispatchAttemptsAndScoresAreRecordedForSupport(t *testing.T) {
	// Document 40: every assignment must be explainable retrospectively. The
	// inputs are volatile, so the explanation is captured with the decision.
	h := newDispatchHarness(t)
	ctx := context.Background()
	job := h.aSearchingJob(t)
	driverID := h.anApprovedDriver(t)

	cfg, err := h.store.Config(ctx, "RIDE")
	if err != nil {
		t.Fatal(err)
	}
	scored := dispatch.Score([]dispatch.Candidate{
		{DriverID: driverID, DistanceMeters: 800, ETASeconds: 200, ETAKnown: true},
	}, cfg.Weights, time.Now())

	if _, err := h.store.RecordAttempt(ctx, job.ID, 1, 2000, 5, 1, "OFFERED", cfg, scored); err != nil {
		t.Fatal(err)
	}

	explanation, err := h.store.Explain(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(explanation) == 0 {
		t.Fatal("no explanation recorded")
	}
	entry := explanation[0]
	if entry["outcome"] != "OFFERED" || entry["radius_meters"] != 2000 {
		t.Fatalf("attempt not recorded correctly: %+v", entry)
	}
	factors, ok := entry["factors"].(map[string]any)
	if !ok || factors["eta_score"] == nil {
		t.Fatalf("score factors were not stored: %+v", entry)
	}
	// The versions that produced the decision, so a later tuning change is
	// distinguishable from a bug.
	if entry["strategy_version"] == nil || entry["score_version"] == nil {
		t.Fatal("strategy and score versions were not recorded")
	}
}

func TestDispatchConfigCarriesTheDefaultsBD03Recommends(t *testing.T) {
	h := newDispatchHarness(t)
	cfg, err := h.store.Config(context.Background(), "RIDE")
	if err != nil {
		t.Fatal(err)
	}
	// ETA dominant, everything else non-zero so no term is dead.
	if cfg.Weights.ETA <= cfg.Weights.Distance {
		t.Errorf("ETA weight %d is not dominant over distance %d", cfg.Weights.ETA, cfg.Weights.Distance)
	}
	for name, weight := range map[string]int{
		"distance": cfg.Weights.Distance, "reliability": cfg.Weights.Reliability,
		"acceptance": cfg.Weights.Acceptance, "idle": cfg.Weights.Idle,
		"capability": cfg.Weights.Capability,
	} {
		if weight <= 0 {
			t.Errorf("%s weight is %d; BD-03 asks that every term be exercised", name, weight)
		}
	}
	if len(cfg.RadiusRings) < 2 {
		t.Errorf("expected expanding rings, got %v", cfg.RadiusRings)
	}
	if cfg.MaxAttempts < 1 {
		t.Error("retries must be bounded but at least one attempt must happen")
	}
}

func TestEventDeduplicationIsDurable(t *testing.T) {
	// Document 46: "NATS delivery may be repeated. Consumers must be
	// idempotent." The dedup has to outlive the process.
	h := newDispatchHarness(t)
	ctx := context.Background()
	eventID := fmt.Sprintf("evt-%d", time.Now().UnixNano())

	first, err := h.store.MarkProcessed(ctx, "dispatch", eventID)
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Fatal("the first delivery was reported as a duplicate")
	}
	again, err := h.store.MarkProcessed(ctx, "dispatch", eventID)
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Fatal("a redelivery was reported as new")
	}
	// A different consumer must still process it — dedup is per consumer.
	other, err := h.store.MarkProcessed(ctx, "analytics", eventID)
	if err != nil {
		t.Fatal(err)
	}
	if !other {
		t.Fatal("a second consumer was blocked by the first consumer's dedup")
	}
}

func TestConcurrentDeduplicationAdmitsOneProcessor(t *testing.T) {
	h := newDispatchHarness(t)
	eventID := fmt.Sprintf("evt-%d", time.Now().UnixNano())

	const racers = 8
	results := make(chan bool, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start
			first, err := h.store.MarkProcessed(context.Background(), "dispatch", eventID)
			if err != nil {
				t.Error(err)
			}
			results <- first
		}()
	}
	close(start)

	var firsts int
	for i := 0; i < racers; i++ {
		if <-results {
			firsts++
		}
	}
	if firsts != 1 {
		t.Fatalf("%d concurrent deliveries were each treated as the first", firsts)
	}
}
