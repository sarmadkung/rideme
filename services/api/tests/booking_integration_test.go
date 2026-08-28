//go:build integration

package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/booking"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/internal/settings"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

type bookingHarness struct {
	service  *booking.Service
	store    *booking.Store
	jobs     *jobs.Store
	pool     *pgxpool.Pool
	settings *settings.Store
	clock    time.Time
}

func newBookingHarness(t *testing.T) *bookingHarness {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	h := &bookingHarness{
		store: booking.NewStore(pool),
		jobs:  jobs.NewStore(pool),
		pool:  pool,
		clock: time.Now().UTC(),
	}
	now := func() time.Time { return h.clock }
	h.settings = settings.NewStore(pool)
	h.service = booking.NewService(h.jobs, h.store, pricing.NewEngine(now), routing.NewService(), h.settings, now)
	return h
}

func (h *bookingHarness) aUser(t *testing.T) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9236' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// aTariff installs test pricing configuration. These numbers exist only in the
// test: the platform ships no tariffs, because rates are the owner's per
// market. What BD-01, BD-02 and BD-05 decided are the platform-wide rules —
// the cancellation fee, the surge ceiling and the commission — not these
// per-city fares.
func (h *bookingHarness) aTariff(t *testing.T, city string) {
	t.Helper()
	if _, err := h.store.SaveTariff(context.Background(), pricing.Tariff{
		JobType: "RIDE", VehicleType: "CAR", City: city, Version: 1,
		Currency: money.PKR, BaseMinor: 5000, PerKMMinor: 3000,
		PerMinuteMinor: 200, ServiceFeeMinor: 1000, MinimumFareMinor: 12000,
	}); err != nil {
		t.Fatal(err)
	}
}

func rideStops() []jobs.Stop {
	return []jobs.Stop{
		{Sequence: 0, Type: jobs.StopPickup, Location: jobs.Coordinate{Latitude: 31.5204, Longitude: 74.3587}},
		{Sequence: 1, Type: jobs.StopDropoff, Location: jobs.Coordinate{Latitude: 31.5880, Longitude: 74.3150}},
	}
}

func (h *bookingHarness) quoteFor(t *testing.T, userID, city string) booking.Quote {
	t.Helper()
	quote, err := h.service.Quote(context.Background(), booking.QuoteRequest{
		JobType: jobs.TypeRide, VehicleType: "CAR", City: city,
		Stops: rideStops(), RequestedBy: userID,
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	return quote
}

func TestQuoteThenConfirmProducesAPricedJob(t *testing.T) {
	// Document 034's flow end to end: requirements → route → pricing → quote →
	// confirmation.
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)

	quote := h.quoteFor(t, userID, city)
	if quote.Total.Minor <= 0 {
		t.Fatalf("quote total = %d", quote.Total.Minor)
	}
	if quote.Job.RouteConfidence == "" {
		t.Fatal("the quote does not say how its route was obtained")
	}

	job, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusRequested {
		t.Fatalf("status = %s", job.Status)
	}

	// Price lock: the confirmed amount is stored and immutable.
	locked, version, err := h.store.LockedPrice(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if locked.Minor != quote.Total.Minor {
		t.Fatalf("locked %d but quoted %d", locked.Minor, quote.Total.Minor)
	}
	if version != 1 {
		t.Fatalf("pricing version = %d", version)
	}
}

func TestAChangedTariffDoesNotChangeAConfirmedPrice(t *testing.T) {
	// Document 034: "Historical prices must not be recomputed from current
	// configuration."
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)

	quote := h.quoteFor(t, userID, city)
	job, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Rates double.
	if _, err := h.store.SaveTariff(ctx, pricing.Tariff{
		JobType: "RIDE", VehicleType: "CAR", City: city, Version: 2,
		Currency: money.PKR, BaseMinor: 10000, PerKMMinor: 6000,
		PerMinuteMinor: 400, ServiceFeeMinor: 2000,
	}); err != nil {
		t.Fatal(err)
	}

	locked, version, err := h.store.LockedPrice(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if locked.Minor != quote.Total.Minor || version != 1 {
		t.Fatalf("the locked price moved with the tariff: %d v%d, want %d v1",
			locked.Minor, version, quote.Total.Minor)
	}

	// A new quote does use the new rates.
	fresh := h.quoteFor(t, userID, city)
	if fresh.Total.Minor <= quote.Total.Minor {
		t.Fatalf("a new quote did not pick up the new tariff: %d vs %d", fresh.Total.Minor, quote.Total.Minor)
	}
}

func TestAnExpiredQuoteCannotBeConfirmed(t *testing.T) {
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)

	quote := h.quoteFor(t, userID, city)
	h.clock = h.clock.Add(pricing.QuoteTTL + time.Minute)

	_, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if err == nil {
		t.Fatal("an expired quote was confirmed")
	}
	if code := httpx.AsError(err).Code; code != httpx.CodeConflict {
		t.Fatalf("code = %s, want conflict", code)
	}
}

func TestAQuoteCannotBeConfirmedByAnotherCustomer(t *testing.T) {
	// Without the ownership check, one customer books at another's locked-in
	// price (document 035 requires quote ownership be verified).
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)

	owner := h.aUser(t)
	stranger := h.aUser(t)
	quote := h.quoteFor(t, owner, city)

	_, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: stranger, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if code := httpx.AsError(err).Code; code != httpx.CodeForbidden {
		t.Fatalf("code = %s, want forbidden", code)
	}
}

func TestAQuoteIsSingleUse(t *testing.T) {
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)
	quote := h.quoteFor(t, userID, city)

	if _, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	}); err != nil {
		t.Fatal(err)
	}
	// Reusing it would give two jobs one locked price.
	if _, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	}); err == nil {
		t.Fatal("a quote was used twice")
	}
}

func TestARetriedCreateReturnsTheSameJob(t *testing.T) {
	// Document 035 requires an Idempotency-Key; document 185 requires retries
	// produce no duplicate effects. A customer whose network dropped must not
	// find two rides on the way.
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)
	quote := h.quoteFor(t, userID, city)

	req := booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide,
		Stops: rideStops(), IdempotencyKey: "key-" + time.Now().Format("150405.000000"),
	}
	first, err := h.service.Create(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.service.Create(ctx, req)
	if err != nil {
		t.Fatalf("the retry failed: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("a retry created a second job: %s then %s", first.ID, second.ID)
	}

	var count int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM jobs WHERE requester_user_id = $1`, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("%d jobs exist for this customer, want 1", count)
	}
}

func TestAKeyReusedWithADifferentRequestIsRefused(t *testing.T) {
	// Replaying the first response would silently discard the second request.
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)

	key := "key-" + time.Now().Format("150405.000000")
	first := h.quoteFor(t, userID, city)
	if _, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: first.ID, RequesterID: userID, JobType: jobs.TypeRide,
		Stops: rideStops(), IdempotencyKey: key,
	}); err != nil {
		t.Fatal(err)
	}

	second := h.quoteFor(t, userID, city)
	_, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: second.ID, RequesterID: userID, JobType: jobs.TypeRide,
		Stops: rideStops(), IdempotencyKey: key,
	})
	if err == nil {
		t.Fatal("a key was reused with different content and accepted")
	}
	if code := httpx.AsError(err).Code; code != httpx.CodeConflict {
		t.Fatalf("code = %s, want conflict", code)
	}
}

func TestCancellationTiersFollowDocument005(t *testing.T) {
	cases := map[jobs.Status]booking.CancellationTier{
		jobs.StatusDraft:      booking.TierBeforeAssignment,
		jobs.StatusRequested:  booking.TierBeforeAssignment,
		jobs.StatusSearching:  booking.TierBeforeAssignment,
		jobs.StatusAssigned:   booking.TierAfterAssignment,
		jobs.StatusAccepted:   booking.TierAfterAssignment,
		jobs.StatusArriving:   booking.TierAfterAssignment,
		jobs.StatusAtPickup:   booking.TierAfterArrival,
		jobs.StatusInProgress: booking.TierAfterStart,
	}
	for status, want := range cases {
		if got := booking.TierFor(status); got != want {
			t.Errorf("%s -> %s, want %s", status, got, want)
		}
	}
}

func TestCancellingBeforeAnyDriverIsAssignedIsFree(t *testing.T) {
	// BD-01: the grace window starts at driver acceptance, so a customer whose
	// request never found a driver is never charged, however long they waited.
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)
	quote := h.quoteFor(t, userID, city)

	job, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if err != nil {
		t.Fatal(err)
	}

	cancelled, outcome, err := h.service.Cancel(ctx, job.ID, userID, jobs.ActorCustomer, "changed my mind")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != jobs.StatusCancelled {
		t.Fatalf("status = %s", cancelled.Status)
	}
	if outcome.Tier != booking.TierBeforeAssignment {
		t.Fatalf("tier = %s", outcome.Tier)
	}
	if !outcome.Fee.IsZero() {
		t.Fatalf("a customer whose job never found a driver was charged %s", outcome.Fee)
	}

	// The zero is stored rather than left null: "this was free" and "nobody
	// decided yet" must not read the same in the ledger of cancellations.
	var fee *int64
	if err := h.pool.QueryRow(ctx,
		`SELECT fee_minor FROM job_cancellations WHERE job_id = $1`, job.ID).Scan(&fee); err != nil {
		t.Fatal(err)
	}
	if fee == nil {
		t.Fatal("the cancellation fee was left null rather than recorded as zero")
	}
	if *fee != 0 {
		t.Fatalf("stored fee = %d minor units, want 0", *fee)
	}
}

func TestACustomerCannotCancelSomeoneElsesJob(t *testing.T) {
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	owner, stranger := h.aUser(t), h.aUser(t)
	quote := h.quoteFor(t, owner, city)

	job, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: owner, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.service.Cancel(ctx, job.ID, stranger, jobs.ActorCustomer, ""); err == nil {
		t.Fatal("a stranger cancelled someone else's job")
	}
}

func TestATripInProgressCannotBeCancelled(t *testing.T) {
	// Document 036: "After start -> normal cancellation not permitted."
	if booking.Cancellable(jobs.StatusInProgress) {
		t.Error("a trip in progress is cancellable")
	}
	if booking.Cancellable(jobs.StatusCompleted) {
		t.Error("a completed trip is cancellable")
	}
	if !booking.Cancellable(jobs.StatusSearching) {
		t.Error("a searching job is not cancellable")
	}
}

func TestConcurrentCancellationsProduceOneCancellation(t *testing.T) {
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)
	quote := h.quoteFor(t, userID, city)

	job, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if err != nil {
		t.Fatal(err)
	}

	const racers = 6
	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := h.service.Cancel(ctx, job.ID, userID, jobs.ActorCustomer, "")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var succeeded int
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of %d concurrent cancellations succeeded, want 1", succeeded, racers)
	}

	history, err := h.jobs.History(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var cancellations int
	for _, change := range history {
		if change.To == jobs.StatusCancelled {
			cancellations++
		}
	}
	if cancellations != 1 {
		t.Fatalf("%d cancellations recorded in history", cancellations)
	}
}

func TestDriverCommandsAdvanceTheTripAndAreIdempotent(t *testing.T) {
	// Document 036: "Repeated commands must return the authoritative result
	// without corrupting state." A driver tapping "arrived" twice on a flaky
	// connection gets the job back, not an error.
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)
	quote := h.quoteFor(t, userID, city)

	job, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Stand in for Phase 8: move the job to ACCEPTED with a driver attached.
	var driverUser, driverID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO users (phone) VALUES ('+9237' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&driverUser); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO drivers (user_id) VALUES ($1) RETURNING id::text`, driverUser).Scan(&driverID); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ from, to jobs.Status }{
		{jobs.StatusRequested, jobs.StatusSearching},
		{jobs.StatusSearching, jobs.StatusAssigned},
		{jobs.StatusAssigned, jobs.StatusAccepted},
	} {
		if _, err := h.jobs.Transition(ctx, job.ID, step.from, step.to, jobs.Actor{Type: jobs.ActorSystem}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.pool.Exec(ctx, `UPDATE jobs SET assigned_driver_id = $2 WHERE id = $1`, job.ID, driverID); err != nil {
		t.Fatal(err)
	}

	for _, cmd := range []booking.Command{booking.CommandArrive, booking.CommandStart, booking.CommandComplete} {
		first, err := h.service.Execute(ctx, job.ID, driverID, cmd)
		if err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		repeat, err := h.service.Execute(ctx, job.ID, driverID, cmd)
		if err != nil {
			t.Fatalf("%s repeated: %v", cmd, err)
		}
		if repeat.Status != first.Status {
			t.Fatalf("%s was not idempotent: %s then %s", cmd, first.Status, repeat.Status)
		}
	}

	final, err := h.jobs.ByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != jobs.StatusCompleted {
		t.Fatalf("final status = %s, want COMPLETED", final.Status)
	}
	// Completing through IN_PROGRESS must still pass through AT_DROPOFF —
	// a trip cannot complete without arriving.
	history, err := h.jobs.History(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sawDropoff bool
	for _, change := range history {
		if change.To == jobs.StatusAtDropoff {
			sawDropoff = true
		}
	}
	if !sawDropoff {
		t.Fatal("the trip completed without passing through AT_DROPOFF")
	}
}

func TestADriverCannotCommandAJobTheyDoNotHold(t *testing.T) {
	// Otherwise any driver could complete any trip.
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)
	userID := h.aUser(t)
	quote := h.quoteFor(t, userID, city)

	job, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.service.Execute(ctx, job.ID, "00000000-0000-0000-0000-000000000000", booking.CommandArrive)
	if code := httpx.AsError(err).Code; code != httpx.CodeForbidden {
		t.Fatalf("code = %s, want forbidden", code)
	}
}

func TestAServiceWithNoTariffIsRefusedNotGuessed(t *testing.T) {
	// A quote with no configured rate would be a number this platform invented.
	h := newBookingHarness(t)
	_, err := h.service.Quote(context.Background(), booking.QuoteRequest{
		JobType: jobs.TypeRide, VehicleType: "CAR",
		City:  "NOWHERE-" + fmt.Sprint(time.Now().UnixNano()),
		Stops: rideStops(), RequestedBy: h.aUser(t),
	})
	if err == nil {
		t.Fatal("a job was priced with no tariff configured")
	}
}

// --- cancellation fees (BD-01) -----------------------------------------------

// anAcceptedJob books a ride and walks it to ACCEPTED with a real driver, so a
// cancellation has an acceptance timestamp to measure the grace window from.
func (h *bookingHarness) anAcceptedJob(t *testing.T, city string) (jobs.Job, time.Time) {
	t.Helper()
	ctx := context.Background()
	userID := h.aUser(t)
	quote := h.quoteFor(t, userID, city)

	job, err := h.service.Create(ctx, booking.CreateRequest{
		QuoteID: quote.ID, RequesterID: userID, JobType: jobs.TypeRide, Stops: rideStops(),
	})
	if err != nil {
		t.Fatal(err)
	}

	var driverID string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO drivers (user_id, verification_status, status)
		 VALUES ($1, 'APPROVED', 'AVAILABLE') RETURNING id::text`, h.aUser(t)).Scan(&driverID); err != nil {
		t.Fatal(err)
	}

	if _, err := h.jobs.Transition(ctx, job.ID, jobs.StatusRequested, jobs.StatusSearching,
		jobs.Actor{Type: jobs.ActorSystem}, nil); err != nil {
		t.Fatal(err)
	}
	offer, err := h.jobs.Offer(ctx, jobs.Assignment{JobID: job.ID, DriverID: driverID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.jobs.RespondToAssignment(ctx, offer.ID,
		jobs.AssignmentOffered, jobs.AssignmentAccepted); err != nil {
		t.Fatal(err)
	}
	if _, err := h.jobs.Transition(ctx, job.ID, jobs.StatusSearching, jobs.StatusAssigned,
		jobs.Actor{Type: jobs.ActorSystem}, nil); err != nil {
		t.Fatal(err)
	}
	moved, err := h.jobs.Transition(ctx, job.ID, jobs.StatusAssigned, jobs.StatusAccepted,
		jobs.Actor{Type: jobs.ActorDriver, ID: driverID}, nil)
	if err != nil {
		t.Fatal(err)
	}

	live, err := h.jobs.LiveAssignment(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if live.AcceptedAt == nil {
		t.Fatal("the assignment records no acceptance time")
	}
	return moved, *live.AcceptedAt
}

func TestCancellingInsideTheFreeWindowCostsNothing(t *testing.T) {
	// BD-01: two free minutes from the moment the driver accepted.
	h := newBookingHarness(t)
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)

	job, acceptedAt := h.anAcceptedJob(t, city)
	h.clock = acceptedAt.Add(90 * time.Second)

	_, outcome, err := h.service.Cancel(context.Background(), job.ID,
		job.RequesterUserID, jobs.ActorCustomer, "changed my mind")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Tier != booking.TierAfterAssignment {
		t.Fatalf("tier = %s, want AFTER_ASSIGNMENT", outcome.Tier)
	}
	if !outcome.Fee.IsZero() {
		t.Fatalf("cancelling 90 seconds after acceptance charged %s", outcome.Fee)
	}
}

func TestCancellingAfterTheFreeWindowCostsOneHundredRupees(t *testing.T) {
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")
	h.aTariff(t, city)

	job, acceptedAt := h.anAcceptedJob(t, city)
	h.clock = acceptedAt.Add(5 * time.Minute)

	_, outcome, err := h.service.Cancel(ctx, job.ID,
		job.RequesterUserID, jobs.ActorCustomer, "changed my mind")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Fee.Minor != 10000 {
		t.Fatalf("fee = %d minor units, want the decided 10000", outcome.Fee.Minor)
	}
	if outcome.Fee.Currency != money.PKR {
		t.Fatalf("fee currency = %s", outcome.Fee.Currency)
	}

	// What the customer was told and what was recorded must agree.
	var stored int64
	if err := h.pool.QueryRow(ctx,
		`SELECT fee_minor FROM job_cancellations WHERE job_id = $1`, job.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != outcome.Fee.Minor {
		t.Fatalf("the customer was charged %d but %d was recorded", outcome.Fee.Minor, stored)
	}
}
