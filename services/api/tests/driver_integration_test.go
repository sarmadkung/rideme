//go:build integration

package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/sarmadkung/rideme/services/api/internal/driver"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
)

type driverHarness struct {
	service   *driver.Service
	providers *providers.Store
	tracking  *tracking.Store
	jobs      *jobs.Store
	pool      *pgxpool.Pool
	clock     time.Time
}

func newDriverHarness(t *testing.T) *driverHarness {
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

	h := &driverHarness{
		providers: providers.NewStore(pool),
		tracking:  tracking.NewStore(pool, client),
		jobs:      jobs.NewStore(pool),
		pool:      pool,
		clock:     time.Now().UTC(),
	}
	h.service = driver.NewService(h.providers, h.tracking, h.jobs,
		tracking.DefaultLimits(), func() time.Time { return h.clock })
	return h
}

func (h *driverHarness) aUser(t *testing.T) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9239' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// aDriver creates a verified driver with an active vehicle — the state a real
// driver reaches before they can work.
func (h *driverHarness) aDriver(t *testing.T, withVehicle bool) (userID, driverID string) {
	t.Helper()
	ctx := context.Background()
	userID = h.aUser(t)

	if err := h.pool.QueryRow(ctx,
		`INSERT INTO drivers (user_id, verification_status, status)
		 VALUES ($1, 'APPROVED', 'OFFLINE') RETURNING id::text`, userID).Scan(&driverID); err != nil {
		t.Fatal(err)
	}
	if !withVehicle {
		return userID, driverID
	}

	var vehicleID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO vehicles (owner_user_id, type, plate_number, verification_status)
		 VALUES ($1, 'CAR', 'DRV-' || floor(random()*100000000)::text, 'VERIFIED')
		 RETURNING id::text`, userID).Scan(&vehicleID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx,
		`UPDATE drivers SET active_vehicle_id = $2 WHERE id = $1`, driverID, vehicleID); err != nil {
		t.Fatal(err)
	}
	return userID, driverID
}

func lahore() tracking.Fix {
	return tracking.Fix{Lat: 31.5204, Lon: 74.3587}
}

func TestGoingOnlineMakesADriverVisibleToDispatch(t *testing.T) {
	// The two halves must happen together. A driver marked AVAILABLE who is
	// not in the geo pool believes they are working and is never offered
	// anything, because the candidate search reads the pool, not the column.
	h := newDriverHarness(t)
	ctx := context.Background()
	userID, driverID := h.aDriver(t, true)

	d, err := h.service.GoOnline(ctx, userID, lahore())
	if err != nil {
		t.Fatalf("GoOnline: %v", err)
	}
	if d.Status != providers.StatusAvailable {
		t.Fatalf("status = %s, want AVAILABLE", d.Status)
	}

	nearby, err := h.tracking.Nearby(ctx, 31.5204, 74.3587, 1000, 50)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, n := range nearby {
		if n.DriverID == driverID {
			found = true
		}
	}
	if !found {
		t.Fatal("the driver is AVAILABLE but dispatch cannot see them")
	}
}

func TestGoingOnlineWithoutAVehicleIsRefused(t *testing.T) {
	// Dispatch matches jobs to vehicle capabilities. A driver with no vehicle
	// can never be offered anything, and would sit online wondering why.
	h := newDriverHarness(t)
	userID, _ := h.aDriver(t, false)

	if _, err := h.service.GoOnline(context.Background(), userID, lahore()); !errors.Is(err, driver.ErrNoVehicle) {
		t.Fatalf("a driver with no vehicle went online: %v", err)
	}
}

func TestGoingOnlineRequiresAUsablePosition(t *testing.T) {
	// "Online" with no location is not a state dispatch can use.
	h := newDriverHarness(t)
	userID, _ := h.aDriver(t, true)

	if _, err := h.service.GoOnline(context.Background(), userID, tracking.Fix{Lat: 0, Lon: 0}); err == nil {
		t.Fatal("a driver went online at a null island coordinate")
	}
}

func TestGoingOfflineWithdrawsFromDispatch(t *testing.T) {
	h := newDriverHarness(t)
	ctx := context.Background()
	userID, driverID := h.aDriver(t, true)

	if _, err := h.service.GoOnline(ctx, userID, lahore()); err != nil {
		t.Fatal(err)
	}
	d, err := h.service.GoOffline(ctx, userID)
	if err != nil {
		t.Fatalf("GoOffline: %v", err)
	}
	if d.Status != providers.StatusOffline {
		t.Fatalf("status = %s, want OFFLINE", d.Status)
	}

	nearby, err := h.tracking.Nearby(ctx, 31.5204, 74.3587, 1000, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nearby {
		if n.DriverID == driverID {
			t.Fatal("an offline driver is still offered to dispatch")
		}
	}
}

func TestABatchOfFixesKeepsTheGoodOnes(t *testing.T) {
	// Document 048's model: a phone buffers while offline and sends what it
	// has. One bad coordinate must not cost the driver the rest of the batch.
	h := newDriverHarness(t)
	ctx := context.Background()
	userID, _ := h.aDriver(t, true)
	if _, err := h.service.GoOnline(ctx, userID, lahore()); err != nil {
		t.Fatal(err)
	}

	base := h.clock
	accepted, rejected, err := h.service.ReportLocation(ctx, userID, []tracking.Fix{
		{Lat: 31.5210, Lon: 74.3590, RecordedAt: base.Add(-40 * time.Second)},
		{Lat: 0, Lon: 0, RecordedAt: base.Add(-30 * time.Second)}, // impossible
		{Lat: 31.5220, Lon: 74.3600, RecordedAt: base.Add(-20 * time.Second)},
	})
	if err != nil {
		t.Fatalf("ReportLocation: %v", err)
	}
	if accepted != 2 {
		t.Fatalf("accepted %d fixes, want 2", accepted)
	}
	if len(rejected) != 1 || rejected[0].Reason != tracking.RejectBadCoordinate {
		t.Fatalf("rejections = %+v", rejected)
	}
}

func TestARejectedFixDoesNotBecomeTheBaseline(t *testing.T) {
	// If a discarded fix were used for jump detection, one bad coordinate
	// would reject every good fix after it.
	h := newDriverHarness(t)
	ctx := context.Background()
	userID, _ := h.aDriver(t, true)
	if _, err := h.service.GoOnline(ctx, userID, lahore()); err != nil {
		t.Fatal(err)
	}

	base := h.clock
	accepted, _, err := h.service.ReportLocation(ctx, userID, []tracking.Fix{
		{Lat: 0, Lon: 0, RecordedAt: base.Add(-40 * time.Second)}, // discarded
		{Lat: 31.5210, Lon: 74.3590, RecordedAt: base.Add(-30 * time.Second)},
		{Lat: 31.5215, Lon: 74.3595, RecordedAt: base.Add(-20 * time.Second)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted != 2 {
		t.Fatalf("accepted %d fixes after a rejected first one, want 2", accepted)
	}
}

func TestAnIdleDriverHoldsNothing(t *testing.T) {
	// Holding nothing is the normal state, not an error the app must handle
	// as a failure.
	h := newDriverHarness(t)
	userID, _ := h.aDriver(t, true)

	if _, err := h.service.Current(context.Background(), userID); err == nil {
		t.Fatal("an idle driver was given an assignment")
	}
}

func TestADriverSeesTheOfferTheyWereMade(t *testing.T) {
	h := newDriverHarness(t)
	ctx := context.Background()
	userID, driverID := h.aDriver(t, true)

	job, err := h.jobs.Create(ctx, jobs.Job{
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

	expires := time.Now().UTC().Add(20 * time.Second)
	if _, err := h.jobs.Offer(ctx, jobs.Assignment{
		JobID: job.ID, DriverID: driverID, ExpiresAt: &expires,
	}); err != nil {
		t.Fatal(err)
	}

	current, err := h.service.Current(ctx, userID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if current.Job.ID != job.ID {
		t.Fatalf("assignment points at job %s, want %s", current.Job.ID, job.ID)
	}
	if current.Assignment.Status != jobs.AssignmentOffered {
		t.Fatalf("assignment status = %s", current.Assignment.Status)
	}
	// The countdown is drawn from this. Without it the app would guess the
	// TTL, and a countdown that disagrees with the server is worse than none.
	if current.Assignment.ExpiresAt == nil {
		t.Fatal("the offer carries no expiry, so no countdown can be shown")
	}
}
