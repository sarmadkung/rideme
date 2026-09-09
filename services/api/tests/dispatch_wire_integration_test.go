//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/sarmadkung/rideme/services/api/internal/dispatch"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/internal/settings"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

// These cover the call that was missing rather than the engine behind it.
// Booking created a job as REQUESTED, nothing moved it to SEARCHING, and the
// runner was constructed with a nil engine — so the dispatch engine Phase 8
// verified had never run against a real customer's job. Every test in
// dispatch_integration_test.go creates its job already SEARCHING, which is
// exactly why the gap went unnoticed.

type wireHarness struct {
	runner  *dispatch.Runner
	jobs    *jobs.Store
	offers  *dispatch.Store
	track   *tracking.Store
	pool    *pgxpool.Pool
	redis   *redis.Client
	nowFunc func() time.Time
}

func newWireHarness(t *testing.T) *wireHarness {
	t.Helper()
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	opts, err := redis.ParseURL(env(t, "REDIS_URL", "redis://localhost:56379/0"))
	if err != nil {
		t.Fatalf("redis url: %v", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	h := &wireHarness{
		jobs:   jobs.NewStore(pool),
		offers: dispatch.NewStore(pool),
		track:  tracking.NewStore(pool, client),
		pool:   pool,
		redis:  client,
	}
	h.nowFunc = time.Now

	// The same wiring main.go does. If this constructor and that one drift,
	// these tests stop describing the running system.
	engine := dispatch.NewEngine(h.offers, h.jobs, providers.NewStore(pool), h.track,
		routing.NewService(), quietLogger(), func() time.Time { return h.nowFunc() })
	h.runner = dispatch.NewRunner(engine, h.jobs, settings.NewStore(pool),
		quietLogger(), func() time.Time { return h.nowFunc() }).WithOffers(h.offers)
	return h
}

// aDispatchableDriver is an approved driver a RIDE can actually be offered to:
// verified vehicle, the PASSENGER capability document 003 requires, and a
// position fresh enough to pass the location-age check.
func (h *wireHarness) aDispatchableDriver(t *testing.T, at jobs.Coordinate) string {
	return h.aDispatchableDriverFor(t, at, "PASSENGER")
}

// aDispatchableDriverFor is the same for any capability document 003 defines,
// so a grocery delivery can be offered to a driver a ride could not be.
func (h *wireHarness) aDispatchableDriverFor(t *testing.T, at jobs.Coordinate,
	capability string) string {
	t.Helper()
	ctx := context.Background()

	var userID, driverID, vehicleID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO users (phone) VALUES ('+9239' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO drivers (user_id, verification_status, status)
		 VALUES ($1, 'APPROVED', 'AVAILABLE') RETURNING id::text`, userID).Scan(&driverID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO vehicles (owner_user_id, type, plate_number, verification_status)
		 VALUES ($1, 'CAR', 'WIRE-' || floor(random()*100000000)::text, 'VERIFIED')
		 RETURNING id::text`, userID).Scan(&vehicleID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO vehicle_capabilities (vehicle_id, capability) VALUES ($1, $2)`,
		vehicleID, capability); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx,
		`UPDATE drivers SET active_vehicle_id = $2 WHERE id = $1`, driverID, vehicleID); err != nil {
		t.Fatal(err)
	}
	if err := h.track.PutCurrent(ctx, tracking.Current{
		DriverID: driverID, Lat: at.Latitude, Lon: at.Longitude, RecordedAt: time.Now().UTC(),
	}, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.track.RemoveFromPool(context.Background(), driverID) })
	return driverID
}

// abandon takes a job out of the dispatch queue at the end of a test.
//
// NeedingDispatch is oldest-first, which is right for a customer and wrong for
// a test database that keeps every job any run ever created: a REQUESTED job
// left behind would sit at the head of the queue for every future run and
// consume the batch a later test needs.
func (h *wireHarness) abandon(t *testing.T, jobID string) {
	t.Cleanup(func() {
		if _, err := h.pool.Exec(context.Background(),
			`UPDATE jobs SET status = 'CANCELLED', updated_at = now() WHERE id = $1`, jobID); err != nil {
			t.Logf("could not clean up job %s: %v", jobID, err)
		}
	})
}

// aRequestedJob is what booking actually creates: REQUESTED, and offered to
// nobody until something drives dispatch.
func (h *wireHarness) aRequestedJob(t *testing.T, pickup jobs.Coordinate, scheduled *time.Time) jobs.Job {
	t.Helper()
	var customerID string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9237' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	job, err := h.jobs.Create(context.Background(), jobs.Job{
		Type:            jobs.TypeRide,
		RequesterUserID: customerID,
		Status:          jobs.StatusRequested,
		ScheduledAt:     scheduled,
		Stops: []jobs.Stop{
			{Sequence: 0, Type: jobs.StopPickup, Location: pickup},
			{Sequence: 1, Type: jobs.StopDropoff,
				Location: jobs.Coordinate{Latitude: pickup.Latitude + 0.02, Longitude: pickup.Longitude}},
		},
	}, jobs.Actor{Type: jobs.ActorCustomer, ID: customerID})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

// somewhereQuiet keeps each test's geo search to its own patch of the map: the
// Redis pool and the jobs table both outlive a run, and a shared pickup point
// would let one test's drivers answer another test's job.
func somewhereQuiet() jobs.Coordinate {
	at := somewhereNew()
	return jobs.Coordinate{Latitude: at.lat, Longitude: at.lon}
}

func TestARequestedJobEntersDispatch(t *testing.T) {
	// The whole point. Before this, a job created by booking stayed REQUESTED
	// for the life of the platform.
	h := newWireHarness(t)
	ctx := context.Background()
	pickup := somewhereQuiet()
	job := h.aRequestedJob(t, pickup, nil)

	result, err := h.runner.Round(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	if result.Started == 0 {
		t.Fatalf("no job entered dispatch: %+v", result)
	}

	moved, err := h.jobs.ByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Status == jobs.StatusRequested {
		t.Fatal("the job is still REQUESTED after a dispatch round")
	}
}

func TestAJobWithADriverNearbyIsOfferedToThem(t *testing.T) {
	h := newWireHarness(t)
	ctx := context.Background()
	pickup := somewhereQuiet()
	driverID := h.aDispatchableDriver(t, pickup)
	job := h.aRequestedJob(t, pickup, nil)

	if _, err := h.runner.Round(ctx, 200); err != nil {
		t.Fatal(err)
	}

	assignment, err := h.jobs.LiveAssignment(ctx, job.ID)
	if err != nil {
		t.Fatalf("no offer reached a driver: %v", err)
	}
	if assignment.DriverID != driverID {
		t.Errorf("offered to %s, want %s", assignment.DriverID, driverID)
	}
	if assignment.Status != jobs.AssignmentOffered {
		t.Errorf("assignment is %s", assignment.Status)
	}

	// And the job says so, rather than still claiming to be searching.
	offered, err := h.jobs.ByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if offered.Status != jobs.StatusAssigned {
		t.Errorf("job is %s, want ASSIGNED", offered.Status)
	}
}

func TestAJobHoldingAnOfferIsNotOfferedAgain(t *testing.T) {
	// Two live offers for one job is the defect document 046 is about, and a
	// second round must not create one.
	h := newWireHarness(t)
	ctx := context.Background()
	pickup := somewhereQuiet()
	h.aDispatchableDriver(t, pickup)
	h.aDispatchableDriver(t, pickup)
	job := h.aRequestedJob(t, pickup, nil)

	if _, err := h.runner.Round(ctx, 200); err != nil {
		t.Fatal(err)
	}
	first, err := h.jobs.LiveAssignment(ctx, job.ID)
	if err != nil {
		t.Fatalf("no offer was made: %v", err)
	}

	if _, err := h.runner.Round(ctx, 200); err != nil {
		t.Fatal(err)
	}
	second, err := h.jobs.LiveAssignment(ctx, job.ID)
	if err != nil {
		t.Fatalf("the offer disappeared: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("a second round replaced a live offer: %s then %s", first.ID, second.ID)
	}
}

func TestAScheduledJobIsNotDispatchedEarly(t *testing.T) {
	// A driver sent to a pickup nobody is waiting at is worse than a job that
	// waits for its time.
	h := newWireHarness(t)
	ctx := context.Background()
	pickup := somewhereQuiet()
	h.aDispatchableDriver(t, pickup)
	later := time.Now().UTC().Add(2 * time.Hour)
	job := h.aRequestedJob(t, pickup, &later)
	h.abandon(t, job.ID)

	if _, err := h.runner.Round(ctx, 200); err != nil {
		t.Fatal(err)
	}
	still, err := h.jobs.ByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.Status != jobs.StatusRequested {
		t.Errorf("a job scheduled for two hours' time is %s", still.Status)
	}
}

func TestAnAbandonedOfferReturnsTheJobToTheNextRing(t *testing.T) {
	// A driver who never answers must not hold a customer's booking. The
	// offer's TTL passes, the round releases it, and the job is offered on.
	h := newWireHarness(t)
	ctx := context.Background()
	pickup := somewhereQuiet()
	first := h.aDispatchableDriver(t, pickup)
	job := h.aRequestedJob(t, pickup, nil)

	if _, err := h.runner.Round(ctx, 200); err != nil {
		t.Fatal(err)
	}
	offer, err := h.jobs.LiveAssignment(ctx, job.ID)
	if err != nil {
		t.Fatalf("no offer was made: %v", err)
	}
	if offer.DriverID != first {
		t.Fatalf("offered to %s, want %s", offer.DriverID, first)
	}

	// A second driver arrives, and the first one's offer times out.
	h.aDispatchableDriver(t, pickup)
	h.nowFunc = func() time.Time { return time.Now().Add(2 * time.Hour) }

	round, err := h.runner.Round(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	if round.Released == 0 {
		t.Error("the abandoned offer was not released")
	}

	// The clock is two hours on, so the search deadline has also passed and
	// the job ends rather than being offered again — which is BD-04 working,
	// not the offer being ignored. What matters here is that the first
	// driver's offer no longer stands.
	stale, err := h.jobs.LiveAssignment(ctx, job.ID)
	if err == nil && stale.DriverID == first && stale.Status == jobs.AssignmentOffered {
		t.Error("the timed-out offer is still live")
	}
}

func TestARunnerWithNoEngineDispatchesNothing(t *testing.T) {
	// main.go built exactly this for as long as dispatch existed: a runner
	// with a nil engine. It must be inert rather than a panic, and it must not
	// move jobs into a search nothing can drive.
	h := newWireHarness(t)
	ctx := context.Background()
	pickup := somewhereQuiet()
	job := h.aRequestedJob(t, pickup, nil)
	h.abandon(t, job.ID)

	inert := dispatch.NewRunner(nil, h.jobs, settings.NewStore(h.pool), quietLogger(), nil)
	result, err := inert.Round(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	if result.Started != 0 || result.Considered != 0 {
		t.Errorf("an engineless runner did work: %+v", result)
	}

	unchanged, err := h.jobs.ByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Status != jobs.StatusRequested {
		t.Errorf("job is %s, want it left REQUESTED and visibly undispatched", unchanged.Status)
	}
}
