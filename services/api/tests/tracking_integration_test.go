//go:build integration

package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
)

type trackingHarness struct {
	store *tracking.Store
	pool  *pgxpool.Pool
	redis *redis.Client
}

func newTrackingHarness(t *testing.T) *trackingHarness {
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

	return &trackingHarness{store: tracking.NewStore(pool, client), pool: pool, redis: client}
}

func (h *trackingHarness) aDriver(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	var userID, driverID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO users (phone) VALUES ('+9232' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO drivers (user_id) VALUES ($1) RETURNING id::text`, userID).Scan(&driverID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.store.RemoveFromPool(context.Background(), driverID) })
	return driverID
}

func TestCurrentLocationRoundTripsThroughRedis(t *testing.T) {
	h := newTrackingHarness(t)
	ctx := context.Background()
	driverID := h.aDriver(t)
	recorded := time.Now().UTC().Truncate(time.Millisecond)

	if err := h.store.PutCurrent(ctx, tracking.Current{
		DriverID: driverID, Lat: 31.5204, Lon: 74.3587, RecordedAt: recorded,
	}, true); err != nil {
		t.Fatal(err)
	}

	current, found, err := h.store.Current(ctx, driverID)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if current.Lat != 31.5204 || current.Lon != 74.3587 {
		t.Fatalf("coordinates changed: %+v", current)
	}
	if !current.RecordedAt.Equal(recorded) {
		t.Fatalf("timestamp changed: %v vs %v", current.RecordedAt, recorded)
	}
}

func TestAnUnknownDriverHasNoCurrentLocation(t *testing.T) {
	h := newTrackingHarness(t)
	_, found, err := h.store.Current(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("an unknown driver reported a location")
	}
}

func TestOnlyAvailableDriversEnterTheDispatchPool(t *testing.T) {
	// The pool is dispatch's candidate set. A driver mid-trip or offline in it
	// would be offered a second job.
	h := newTrackingHarness(t)
	ctx := context.Background()
	available := h.aDriver(t)
	busy := h.aDriver(t)
	pickup := tracking.Current{Lat: 31.5204, Lon: 74.3587, RecordedAt: time.Now().UTC()}

	first := pickup
	first.DriverID = available
	if err := h.store.PutCurrent(ctx, first, true); err != nil {
		t.Fatal(err)
	}
	second := pickup
	second.DriverID = busy
	if err := h.store.PutCurrent(ctx, second, false); err != nil {
		t.Fatal(err)
	}

	nearby, err := h.store.Nearby(ctx, 31.5204, 74.3587, 2000, 50)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, d := range nearby {
		seen[d.DriverID] = true
	}
	if !seen[available] {
		t.Fatal("an available driver was not in the dispatch pool")
	}
	if seen[busy] {
		t.Fatal("an unavailable driver was in the dispatch pool")
	}
}

func TestGoingOffMissionRemovesADriverFromThePoolImmediately(t *testing.T) {
	h := newTrackingHarness(t)
	ctx := context.Background()
	driverID := h.aDriver(t)

	if err := h.store.PutCurrent(ctx, tracking.Current{
		DriverID: driverID, Lat: 31.5204, Lon: 74.3587, RecordedAt: time.Now().UTC(),
	}, true); err != nil {
		t.Fatal(err)
	}
	// Accepting a job makes the driver unavailable. Waiting for a TTL to
	// expire would keep offering them work in the meantime.
	if err := h.store.PutCurrent(ctx, tracking.Current{
		DriverID: driverID, Lat: 31.5210, Lon: 74.3590, RecordedAt: time.Now().UTC(),
	}, false); err != nil {
		t.Fatal(err)
	}

	nearby, err := h.store.Nearby(ctx, 31.5204, 74.3587, 2000, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range nearby {
		if d.DriverID == driverID {
			t.Fatal("a driver who accepted a job is still in the pool")
		}
	}
}

func TestNearbyIsBoundedByRadiusAndOrderedByDistance(t *testing.T) {
	h := newTrackingHarness(t)
	ctx := context.Background()
	near := h.aDriver(t)
	far := h.aDriver(t)
	now := time.Now().UTC()

	// Near is ~500m from the pickup; far is in Gujranwala, ~67km away.
	if err := h.store.PutCurrent(ctx, tracking.Current{DriverID: near, Lat: 31.5249, Lon: 74.3587, RecordedAt: now}, true); err != nil {
		t.Fatal(err)
	}
	if err := h.store.PutCurrent(ctx, tracking.Current{DriverID: far, Lat: 32.1877, Lon: 74.1945, RecordedAt: now}, true); err != nil {
		t.Fatal(err)
	}

	nearby, err := h.store.Nearby(ctx, 31.5204, 74.3587, 3000, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range nearby {
		if d.DriverID == far {
			t.Fatal("a driver 67km away was returned within a 3km radius")
		}
		if d.DistanceMeters > 3000 {
			t.Fatalf("%s is %.0fm away, outside the radius", d.DriverID, d.DistanceMeters)
		}
	}

	wide, err := h.store.Nearby(ctx, 31.5204, 74.3587, 100000, 50)
	if err != nil {
		t.Fatal(err)
	}
	var lastDistance float64
	for _, d := range wide {
		if d.DistanceMeters < lastDistance {
			t.Fatal("results were not ordered nearest first")
		}
		lastDistance = d.DistanceMeters
	}
}

func TestHistoryIsWrittenInBatchesAndReadBackInOrder(t *testing.T) {
	h := newTrackingHarness(t)
	ctx := context.Background()
	driverID := h.aDriver(t)
	base := time.Now().UTC().Add(-time.Minute)

	fixes := make([]tracking.Fix, 0, 5)
	for i := 0; i < 5; i++ {
		fixes = append(fixes, tracking.Fix{
			DriverID:   driverID,
			Lat:        31.5204 + float64(i)*0.001,
			Lon:        74.3587,
			RecordedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	written, err := h.store.AppendHistory(ctx, fixes)
	if err != nil {
		t.Fatal(err)
	}
	if written != 5 {
		t.Fatalf("wrote %d rows, want 5", written)
	}

	last, err := h.store.LastFix(ctx, driverID)
	if err != nil {
		t.Fatal(err)
	}
	if last == nil {
		t.Fatal("no last fix found")
	}
	// The newest, not merely any.
	if d := last.Lat - (31.5204 + 4*0.001); d > 0.00001 || d < -0.00001 {
		t.Fatalf("last fix latitude = %v, want the newest", last.Lat)
	}
}

func TestEmptyBatchWritesNothing(t *testing.T) {
	h := newTrackingHarness(t)
	written, err := h.store.AppendHistory(context.Background(), nil)
	if err != nil || written != 0 {
		t.Fatalf("written=%d err=%v", written, err)
	}
}

func TestRetentionPurgeRemovesOldHistoryOnly(t *testing.T) {
	// BD-15 sets the period and is unresolved; the mechanism takes the cutoff
	// as an argument so no default is baked in.
	h := newTrackingHarness(t)
	ctx := context.Background()
	driverID := h.aDriver(t)
	now := time.Now().UTC()

	if _, err := h.store.AppendHistory(ctx, []tracking.Fix{
		{DriverID: driverID, Lat: 31.52, Lon: 74.35, RecordedAt: now.AddDate(0, 0, -90)},
		{DriverID: driverID, Lat: 31.53, Lon: 74.36, RecordedAt: now.Add(-time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}

	cutoff := now.AddDate(0, 0, -30)
	if _, err := h.store.PurgeHistoryBefore(ctx, cutoff); err != nil {
		t.Fatal(err)
	}

	var remaining, old int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM driver_locations WHERE driver_id = $1`, driverID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM driver_locations WHERE driver_id = $1 AND recorded_at < $2`,
		driverID, cutoff).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if old != 0 {
		t.Fatalf("%d rows survived the retention cutoff", old)
	}
	if remaining != 1 {
		t.Fatalf("%d rows remain, want the recent one only", remaining)
	}
}

func TestTrackingSessionIsUniquePerLiveJob(t *testing.T) {
	h := newTrackingHarness(t)
	ctx := context.Background()
	driverID := h.aDriver(t)

	var requester, jobID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO users (phone) VALUES ('+9233' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&requester); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO jobs (type, requester_user_id) VALUES ('RIDE', $1) RETURNING id::text`,
		requester).Scan(&jobID); err != nil {
		t.Fatal(err)
	}

	first, err := h.store.StartSession(ctx, jobID, driverID)
	if err != nil {
		t.Fatal(err)
	}
	// Restarting tracking on a job already being tracked must not open a
	// second session; two would mean two answers to "who may watch this".
	again, err := h.store.StartSession(ctx, jobID, driverID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Fatal("a second live tracking session was opened for one job")
	}

	if err := h.store.EndSession(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if _, live, err := h.store.LiveSession(ctx, jobID); err != nil || live {
		t.Fatalf("session still live after ending: live=%v err=%v", live, err)
	}
}

func TestLocationAccessIsScopedAndAudited(t *testing.T) {
	// Document 102: unauthorized users cannot access location streams, and
	// privileged access is audited.
	h := newTrackingHarness(t)
	ctx := context.Background()
	driverID := h.aDriver(t)

	var owner, stranger, jobID string
	for _, target := range []*string{&owner, &stranger} {
		if err := h.pool.QueryRow(ctx,
			`INSERT INTO users (phone) VALUES ('+9234' || lpad((floor(random()*100000000))::text, 8, '0'))
			 RETURNING id::text`).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO jobs (type, requester_user_id) VALUES ('RIDE', $1) RETURNING id::text`,
		owner).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.StartSession(ctx, jobID, driverID); err != nil {
		t.Fatal(err)
	}

	if err := h.store.AuthorizeView(ctx, owner, "CUSTOMER", driverID, jobID, tracking.ScopeOwnJob); err != nil {
		t.Fatalf("the job's owner was refused: %v", err)
	}
	if err := h.store.AuthorizeView(ctx, stranger, "CUSTOMER", driverID, jobID, tracking.ScopeOwnJob); !errors.Is(err, tracking.ErrNotPermitted) {
		t.Fatalf("a stranger saw a driver's location: %v", err)
	}

	// Both attempts are on the record, granted and denied alike.
	var granted, denied int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM location_access_log WHERE actor_id = $1 AND granted`, owner).Scan(&granted); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM location_access_log WHERE actor_id = $1 AND NOT granted`, stranger).Scan(&denied); err != nil {
		t.Fatal(err)
	}
	if granted == 0 {
		t.Error("a granted access was not audited")
	}
	if denied == 0 {
		t.Error("a denied access was not audited")
	}
}

func TestAccessEndsWhenTrackingEnds(t *testing.T) {
	// Document 102: customers see "only location needed for active service".
	// Once the job is over, so is the visibility.
	h := newTrackingHarness(t)
	ctx := context.Background()
	driverID := h.aDriver(t)

	var owner, jobID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO users (phone) VALUES ('+9235' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO jobs (type, requester_user_id) VALUES ('RIDE', $1) RETURNING id::text`,
		owner).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.StartSession(ctx, jobID, driverID); err != nil {
		t.Fatal(err)
	}
	if err := h.store.AuthorizeView(ctx, owner, "CUSTOMER", driverID, jobID, tracking.ScopeOwnJob); err != nil {
		t.Fatal(err)
	}

	if err := h.store.EndSession(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if err := h.store.AuthorizeView(ctx, owner, "CUSTOMER", driverID, jobID, tracking.ScopeOwnJob); !errors.Is(err, tracking.ErrNotPermitted) {
		t.Fatalf("a customer could still watch the driver after the job ended: %v", err)
	}
}
