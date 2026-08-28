//go:build integration

package tests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// The job core's important guarantees are enforced by SQL — compare-and-set
// transitions and a partial unique index on live assignments. A mocked store
// would assert nothing about either, so these run against real Postgres.

type jobHarness struct {
	store *jobs.Store
	pool  *pgxpool.Pool
}

func newJobHarness(t *testing.T) *jobHarness {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return &jobHarness{store: jobs.NewStore(pool), pool: pool}
}

// aUser creates a bare user row to hang jobs off.
func (h *jobHarness) aUser(t *testing.T) string {
	t.Helper()
	var id string
	err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9230' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *jobHarness) aDriver(t *testing.T) string {
	t.Helper()
	var id string
	err := h.pool.QueryRow(context.Background(),
		`INSERT INTO drivers (user_id) VALUES ($1) RETURNING id::text`, h.aUser(t)).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *jobHarness) aJob(t *testing.T, status jobs.Status) jobs.Job {
	t.Helper()
	job, err := h.store.Create(context.Background(), jobs.Job{
		Type:            jobs.TypeRide,
		RequesterUserID: h.aUser(t),
		Status:          status,
		Stops: []jobs.Stop{
			{Sequence: 0, Type: jobs.StopPickup, Location: jobs.Coordinate{Latitude: 31.5204, Longitude: 74.3587}, Address: "Lahore"},
			{Sequence: 1, Type: jobs.StopDropoff, Location: jobs.Coordinate{Latitude: 31.5820, Longitude: 74.3294}},
		},
	}, jobs.Actor{Type: jobs.ActorCustomer})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestCreatingAJobStoresStopsAndOpeningHistory(t *testing.T) {
	h := newJobHarness(t)
	ctx := context.Background()

	job := h.aJob(t, jobs.StatusDraft)
	if job.ID == "" || job.Status != jobs.StatusDraft {
		t.Fatalf("unexpected job: %+v", job)
	}

	loaded, err := h.store.ByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Stops) != 2 {
		t.Fatalf("want 2 stops, got %d", len(loaded.Stops))
	}
	// Coordinates must survive the geography round trip. Swapping lat and lon
	// puts a Lahore pickup in the Indian Ocean, and nothing else would notice.
	pickup, _ := loaded.Pickup()
	if d := pickup.Location.Latitude - 31.5204; d > 0.0001 || d < -0.0001 {
		t.Fatalf("latitude did not survive storage: %v", pickup.Location.Latitude)
	}
	if d := pickup.Location.Longitude - 74.3587; d > 0.0001 || d < -0.0001 {
		t.Fatalf("longitude did not survive storage: %v", pickup.Location.Longitude)
	}

	history, err := h.store.History(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].To != jobs.StatusDraft {
		t.Fatalf("creation was not recorded in history: %+v", history)
	}
}

func TestCreationIsAtomic(t *testing.T) {
	// A job whose stops failed to insert is a job with no destination, which
	// dispatch would happily try to serve.
	h := newJobHarness(t)
	ctx := context.Background()

	_, err := h.store.Create(ctx, jobs.Job{
		Type:            jobs.TypeRide,
		RequesterUserID: h.aUser(t),
		Stops: []jobs.Stop{
			{Sequence: 0, Type: jobs.StopPickup, Location: jobs.Coordinate{Latitude: 31.5, Longitude: 74.3}},
			{Sequence: 1, Type: "TELEPORT", Location: jobs.Coordinate{Latitude: 31.6, Longitude: 74.4}},
		},
	}, jobs.Actor{Type: jobs.ActorCustomer})
	if err == nil {
		t.Fatal("an invalid stop type was accepted")
	}

	var orphans int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM jobs WHERE id NOT IN (SELECT DISTINCT job_id FROM job_stops)`).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Fatalf("%d jobs exist with no stops; creation is not atomic", orphans)
	}
}

func TestTransitionRecordsHistoryAndStampsTermination(t *testing.T) {
	h := newJobHarness(t)
	ctx := context.Background()
	job := h.aJob(t, jobs.StatusDraft)

	moved, err := h.store.Transition(ctx, job.ID, jobs.StatusDraft, jobs.StatusQuoted,
		jobs.Actor{Type: jobs.ActorSystem}, map[string]any{"reason": "quote issued"})
	if err != nil {
		t.Fatal(err)
	}
	if moved.Status != jobs.StatusQuoted {
		t.Fatalf("status = %s", moved.Status)
	}
	if moved.TerminatedAt != nil {
		t.Fatal("a live job was stamped as terminated")
	}

	cancelled, err := h.store.Transition(ctx, job.ID, jobs.StatusQuoted, jobs.StatusCancelled,
		jobs.Actor{Type: jobs.ActorCustomer, ID: job.RequesterUserID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.TerminatedAt == nil {
		t.Fatal("a terminal transition did not stamp terminated_at")
	}

	history, err := h.store.History(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("want 3 history rows (create, quote, cancel), got %d", len(history))
	}
	// Document 15 requires previous and new status, actor and metadata.
	if history[1].From != jobs.StatusDraft || history[1].To != jobs.StatusQuoted {
		t.Fatalf("history did not record the transition: %+v", history[1])
	}
	if history[1].Metadata["reason"] != "quote issued" {
		t.Fatalf("metadata was lost: %+v", history[1].Metadata)
	}
	if history[2].Actor.Type != jobs.ActorCustomer {
		t.Fatalf("actor was not recorded: %+v", history[2].Actor)
	}
}

func TestAnIllegalTransitionIsRefusedBeforeItReachesTheDatabase(t *testing.T) {
	h := newJobHarness(t)
	job := h.aJob(t, jobs.StatusDraft)

	_, err := h.store.Transition(context.Background(), job.ID, jobs.StatusDraft, jobs.StatusCompleted,
		jobs.Actor{Type: jobs.ActorSystem}, nil)
	if err == nil {
		t.Fatal("DRAFT -> COMPLETED was accepted")
	}

	loaded, err := h.store.ByID(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != jobs.StatusDraft {
		t.Fatalf("the job moved anyway: %s", loaded.Status)
	}
}

func TestConcurrentTransitionsProduceOneWinner(t *testing.T) {
	// Two actors racing on one job: a customer cancelling as dispatch assigns.
	// Without compare-and-set both writes land and the second silently
	// overwrites the first — a cancelled job with a driver on the way.
	h := newJobHarness(t)
	ctx := context.Background()
	job := h.aJob(t, jobs.StatusSearching)

	const racers = 8
	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})

	for i := 0; i < racers; i++ {
		to := jobs.StatusAssigned
		if i%2 == 1 {
			to = jobs.StatusCancelled
		}
		wg.Add(1)
		go func(to jobs.Status) {
			defer wg.Done()
			<-start
			_, err := h.store.Transition(ctx, job.ID, jobs.StatusSearching, to,
				jobs.Actor{Type: jobs.ActorSystem}, nil)
			results <- err
		}(to)
	}
	close(start)
	wg.Wait()
	close(results)

	var succeeded int
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, jobs.ErrStaleTransition) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of %d concurrent transitions succeeded, want exactly 1", succeeded, racers)
	}

	history, err := h.store.History(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("want 2 history rows, got %d — a losing transition was recorded", len(history))
	}
}

func TestTwoDriversCannotHoldOneJob(t *testing.T) {
	// The invariant Phase 8 is built on. An application-level check between a
	// SELECT and an INSERT has a window; the partial unique index does not.
	h := newJobHarness(t)
	ctx := context.Background()
	job := h.aJob(t, jobs.StatusSearching)

	const racers = 10
	drivers := make([]string, racers)
	for i := range drivers {
		drivers[i] = h.aDriver(t)
	}

	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(driverID string) {
			defer wg.Done()
			<-start
			_, err := h.store.Offer(ctx, jobs.Assignment{JobID: job.ID, DriverID: driverID})
			results <- err
		}(drivers[i])
	}
	close(start)
	wg.Wait()
	close(results)

	var claimed int
	for err := range results {
		switch {
		case err == nil:
			claimed++
		case errors.Is(err, jobs.ErrAlreadyClaimed):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if claimed != 1 {
		t.Fatalf("%d of %d drivers claimed the job, want exactly 1", claimed, racers)
	}
}

func TestARejectedOfferFreesTheJobForAnotherDriver(t *testing.T) {
	h := newJobHarness(t)
	ctx := context.Background()
	job := h.aJob(t, jobs.StatusSearching)

	first, err := h.store.Offer(ctx, jobs.Assignment{JobID: job.ID, DriverID: h.aDriver(t)})
	if err != nil {
		t.Fatal(err)
	}
	// While the first offer is live, nobody else can be offered the job.
	if _, err := h.store.Offer(ctx, jobs.Assignment{JobID: job.ID, DriverID: h.aDriver(t)}); !errors.Is(err, jobs.ErrAlreadyClaimed) {
		t.Fatalf("a second live offer was created: %v", err)
	}

	if _, err := h.store.RespondToAssignment(ctx, first.ID, jobs.AssignmentOffered, jobs.AssignmentRejected); err != nil {
		t.Fatal(err)
	}
	// Once rejected, the job is available again — otherwise one declining
	// driver strands the customer.
	if _, err := h.store.Offer(ctx, jobs.Assignment{JobID: job.ID, DriverID: h.aDriver(t)}); err != nil {
		t.Fatalf("the job stayed locked after a rejection: %v", err)
	}
}

func TestOneOfferCannotBeAnsweredTwice(t *testing.T) {
	h := newJobHarness(t)
	ctx := context.Background()
	job := h.aJob(t, jobs.StatusSearching)

	offer, err := h.store.Offer(ctx, jobs.Assignment{JobID: job.ID, DriverID: h.aDriver(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.RespondToAssignment(ctx, offer.ID, jobs.AssignmentOffered, jobs.AssignmentAccepted); err != nil {
		t.Fatal(err)
	}
	// A retried accept, or an accept racing a timeout, must not re-open it.
	if _, err := h.store.RespondToAssignment(ctx, offer.ID, jobs.AssignmentOffered, jobs.AssignmentRejected); !errors.Is(err, jobs.ErrStaleTransition) {
		t.Fatalf("an already-answered offer was answered again: %v", err)
	}
}

func TestQuotesRoundTripAsIntegerMinorUnits(t *testing.T) {
	h := newJobHarness(t)
	ctx := context.Background()

	low := money.MustNew(180000, money.PKR)
	high := money.MustNew(210000, money.PKR)
	quote, err := h.store.CreateQuote(ctx, jobs.Quote{
		JobType:   jobs.TypeRide,
		Amount:    money.MustNew(195000, money.PKR),
		Low:       &low,
		High:      &high,
		Breakdown: map[string]any{"base": 5000, "distance": 190000},
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := h.store.QuoteByID(ctx, quote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Amount.Minor != 195000 || loaded.Amount.Currency != money.PKR {
		t.Fatalf("amount did not survive: %+v", loaded.Amount)
	}
	if loaded.Low == nil || loaded.Low.Minor != 180000 || loaded.High.Minor != 210000 {
		t.Fatalf("range did not survive: %+v %+v", loaded.Low, loaded.High)
	}
	if loaded.Breakdown["base"] == nil {
		t.Fatal("breakdown was lost")
	}
}

func TestTheSchemaRefusesANegativeOrForeignQuote(t *testing.T) {
	h := newJobHarness(t)
	ctx := context.Background()

	if _, err := h.pool.Exec(ctx,
		`INSERT INTO pricing_quotes (job_type, amount_minor) VALUES ('RIDE', -1)`); err == nil {
		t.Error("the schema accepted a negative quote")
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO pricing_quotes (job_type, amount_minor, currency) VALUES ('RIDE', 100, 'USD')`); err == nil {
		t.Error("the schema accepted a currency the platform does not support")
	}
	// A range whose low exceeds its high is not a range.
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO pricing_quotes (job_type, amount_minor, low_minor, high_minor) VALUES ('RIDE', 100, 200, 100)`); err == nil {
		t.Error("the schema accepted an inverted price range")
	}
}

func TestTheSchemaRefusesAnUndocumentedStatusOrType(t *testing.T) {
	// The constraints are the last line of defence if application validation
	// is bypassed — a migration, a script, a future module.
	h := newJobHarness(t)
	ctx := context.Background()
	userID := h.aUser(t)

	if _, err := h.pool.Exec(ctx,
		`INSERT INTO jobs (type, requester_user_id) VALUES ('TAXI', $1)`, userID); err == nil {
		t.Error("the schema accepted an undocumented job type")
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO jobs (type, requester_user_id, status) VALUES ('RIDE', $1, 'PENDING')`, userID); err == nil {
		t.Error("the schema accepted an undocumented job status")
	}
}

func TestOnlyOnePrimaryVehiclePerDriver(t *testing.T) {
	h := newJobHarness(t)
	ctx := context.Background()
	driverID := h.aDriver(t)

	var first, second string
	for _, target := range []*string{&first, &second} {
		if err := h.pool.QueryRow(ctx,
			`INSERT INTO vehicles (owner_user_id, type, plate_number)
			 VALUES ($1, 'MOTORCYCLE', 'LEA-' || floor(random()*1000000)::text) RETURNING id::text`,
			h.aUser(t)).Scan(target); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := h.pool.Exec(ctx,
		`INSERT INTO driver_vehicles (driver_id, vehicle_id, is_primary) VALUES ($1, $2, true)`,
		driverID, first); err != nil {
		t.Fatal(err)
	}
	// "Primary" that can be held twice means nothing to the code picking a
	// default vehicle.
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO driver_vehicles (driver_id, vehicle_id, is_primary) VALUES ($1, $2, true)`,
		driverID, second); err == nil {
		t.Fatal("a driver was given two primary vehicles")
	}
}

func TestListForRequesterPaginatesNewestFirst(t *testing.T) {
	h := newJobHarness(t)
	ctx := context.Background()
	userID := h.aUser(t)

	for i := 0; i < 3; i++ {
		if _, err := h.store.Create(ctx, jobs.Job{
			Type:            jobs.TypeParcel,
			RequesterUserID: userID,
			Stops: []jobs.Stop{
				{Sequence: 0, Type: jobs.StopPickup, Location: jobs.Coordinate{Latitude: 31.5, Longitude: 74.3}},
			},
		}, jobs.Actor{Type: jobs.ActorCustomer}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // distinct created_at for a stable order
	}

	page, err := h.store.ListForRequester(ctx, userID, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 {
		t.Fatalf("want 2 jobs, got %d", len(page))
	}
	if page[0].CreatedAt.Before(page[1].CreatedAt) {
		t.Fatal("jobs were not newest first")
	}

	next, err := h.store.ListForRequester(ctx, userID, &page[1].CreatedAt, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 {
		t.Fatalf("want 1 job on the second page, got %d", len(next))
	}
	// The cursor must not repeat a row it already returned.
	if next[0].ID == page[1].ID {
		t.Fatal("the cursor repeated a row")
	}
}
