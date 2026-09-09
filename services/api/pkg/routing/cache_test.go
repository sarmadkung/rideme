package routing_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

// countingProvider answers a fixed route and records how often it was asked.
//
// Guarded because the concurrency test calls it from many goroutines at once,
// and an unsynchronised counter there would report a race in the test rather
// than in the code under test.
type countingProvider struct {
	mu    sync.Mutex
	calls int
	route routing.Route
	err   error
}

func (p *countingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *countingProvider) Name() string    { return "counting" }
func (p *countingProvider) Version() string { return "1" }

func (p *countingProvider) Route(context.Context, routing.Point, routing.Point, routing.Options) (routing.Route, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return routing.Route{}, p.err
	}
	return p.route, nil
}

func (p *countingProvider) Matrix(_ context.Context, origins, destinations []routing.Point, _ routing.Options) (routing.Matrix, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return routing.Matrix{Entries: []routing.MatrixEntry{{DurationSeconds: 60}}}, nil
}

func aLiveRoute() routing.Route {
	return routing.Route{
		DistanceMeters:  12000,
		DurationSeconds: 1680,
		Confidence:      routing.ConfidenceLive,
		Provider:        "counting",
	}
}

func TestASecondQuoteForTheSameTripIsNotBilledAgain(t *testing.T) {
	// Document 104 asks directly to "avoid duplicate route requests". Two
	// customers quoting the same trip a minute apart is the common case.
	provider := &countingProvider{route: aLiveRoute()}
	caching := routing.NewCachingProvider(provider, routing.NewMemoryCache(nil), 0)

	for range 5 {
		if _, err := caching.Route(context.Background(), lahoreFort, gulberg, routing.Options{}); err != nil {
			t.Fatal(err)
		}
	}
	if provider.count() != 1 {
		t.Errorf("provider called %d times, want 1", provider.count())
	}
}

func TestAReusedRouteIsNotPresentedAsAFreshMeasurement(t *testing.T) {
	// Document 96: "Never present a fallback as exact." A caller that cannot
	// tell a live route from a five-minute-old one will show both as an
	// arrival time, and one of them is behind.
	provider := &countingProvider{route: aLiveRoute()}
	caching := routing.NewCachingProvider(provider, routing.NewMemoryCache(nil), 0)

	first, err := caching.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Confidence != routing.ConfidenceLive {
		t.Errorf("first confidence = %q, want live", first.Confidence)
	}

	second, err := caching.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Confidence != routing.ConfidenceCached {
		t.Errorf("second confidence = %q, want cached", second.Confidence)
	}
	if second.DistanceMeters != first.DistanceMeters {
		t.Errorf("distance changed on reuse: %d then %d", first.DistanceMeters, second.DistanceMeters)
	}
}

func TestARouteIsNotReusedPastItsTTL(t *testing.T) {
	// TrafficDurationSeconds is a statement about right now. An hour old it is
	// not stale, it is wrong.
	now := time.Now()
	clock := func() time.Time { return now }
	provider := &countingProvider{route: aLiveRoute()}
	caching := routing.NewCachingProvider(provider, routing.NewMemoryCache(clock), time.Minute)

	ctx := context.Background()
	if _, err := caching.Route(ctx, lahoreFort, gulberg, routing.Options{}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(61 * time.Second)
	if _, err := caching.Route(ctx, lahoreFort, gulberg, routing.Options{}); err != nil {
		t.Fatal(err)
	}
	if provider.count() != 2 {
		t.Errorf("provider called %d times, want 2 — the entry should have expired", provider.count())
	}
}

func TestDifferentQuestionsDoNotShareAnAnswer(t *testing.T) {
	provider := &countingProvider{route: aLiveRoute()}
	caching := routing.NewCachingProvider(provider, routing.NewMemoryCache(nil), 0)
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"driving", func() error {
			_, err := caching.Route(ctx, lahoreFort, gulberg, routing.Options{Mode: routing.ModeDriving})
			return err
		}},
		// A truck and a car asking about the same two points are asking
		// different questions.
		{"motorcycle", func() error {
			_, err := caching.Route(ctx, lahoreFort, gulberg, routing.Options{Mode: routing.ModeMotorcycle})
			return err
		}},
		{"reversed", func() error {
			_, err := caching.Route(ctx, gulberg, lahoreFort, routing.Options{})
			return err
		}},
		{"elsewhere", func() error {
			_, err := caching.Route(ctx, lahoreFort, gujranwala, routing.Options{})
			return err
		}},
	}
	for _, c := range cases {
		if err := c.call(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
	if provider.count() != len(cases) {
		t.Errorf("provider called %d times, want %d — entries collided", provider.count(), len(cases))
	}
}

func TestCoordinatesAreNotRoundedIntoADifferentTrip(t *testing.T) {
	// The key rounds to about 11 metres, finer than a GPS fix. Rounding harder
	// would raise the hit rate by quoting a trip that starts somewhere the
	// customer is not.
	provider := &countingProvider{route: aLiveRoute()}
	caching := routing.NewCachingProvider(provider, routing.NewMemoryCache(nil), 0)
	ctx := context.Background()

	if _, err := caching.Route(ctx, lahoreFort, gulberg, routing.Options{}); err != nil {
		t.Fatal(err)
	}
	// ~500 m away: a different pickup, and it must be priced as one.
	elsewhere := routing.Point{Lat: lahoreFort.Lat + 0.0045, Lon: lahoreFort.Lon}
	if _, err := caching.Route(ctx, elsewhere, gulberg, routing.Options{}); err != nil {
		t.Fatal(err)
	}
	if provider.count() != 2 {
		t.Errorf("provider called %d times, want 2 — a 500m difference was rounded away", provider.count())
	}
}

func TestAnUpgradedProviderDoesNotServeThePreviousOnesAnswers(t *testing.T) {
	cache := routing.NewMemoryCache(nil)
	first := &countingProvider{route: aLiveRoute()}
	if _, err := routing.NewCachingProvider(first, cache, 0).
		Route(context.Background(), lahoreFort, gulberg, routing.Options{}); err != nil {
		t.Fatal(err)
	}

	// Same provider name, new version — it routes differently now.
	upgraded := &versionedProvider{countingProvider: &countingProvider{route: aLiveRoute()}, version: "2"}
	if _, err := routing.NewCachingProvider(upgraded, cache, 0).
		Route(context.Background(), lahoreFort, gulberg, routing.Options{}); err != nil {
		t.Fatal(err)
	}
	if upgraded.count() != 1 {
		t.Errorf("upgraded provider called %d times, want 1", upgraded.count())
	}
}

type versionedProvider struct {
	*countingProvider
	version string
}

func (p *versionedProvider) Version() string { return p.version }

func TestAFailureIsNotRemembered(t *testing.T) {
	// A rejected key or a rate limit is a condition that clears. Caching it
	// would extend the outage past its end.
	failing := errors.New("rate limited")
	provider := &countingProvider{route: aLiveRoute(), err: failing}
	caching := routing.NewCachingProvider(provider, routing.NewMemoryCache(nil), 0)
	ctx := context.Background()

	if _, err := caching.Route(ctx, lahoreFort, gulberg, routing.Options{}); !errors.Is(err, failing) {
		t.Fatalf("err = %v, want the provider's", err)
	}
	provider.mu.Lock()
	provider.err = nil
	provider.mu.Unlock()
	route, err := caching.Route(ctx, lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatalf("the recovered provider was not retried: %v", err)
	}
	if route.Confidence != routing.ConfidenceLive {
		t.Errorf("confidence = %q, want live", route.Confidence)
	}
}

func TestAScheduledTripIsNeitherServedNorStoredFromTheCache(t *testing.T) {
	// A departure at 6pm is a different question from one leaving now, and its
	// answer must not poison the entries that mean "now".
	provider := &countingProvider{route: aLiveRoute()}
	cache := routing.NewMemoryCache(nil)
	caching := routing.NewCachingProvider(provider, cache, 0)
	ctx := context.Background()
	later := routing.Options{DepartAt: time.Now().Add(6 * time.Hour)}

	if _, err := caching.Route(ctx, lahoreFort, gulberg, later); err != nil {
		t.Fatal(err)
	}
	if _, err := caching.Route(ctx, lahoreFort, gulberg, later); err != nil {
		t.Fatal(err)
	}
	if provider.count() != 2 {
		t.Errorf("provider called %d times, want 2 — a scheduled route was cached", provider.count())
	}
	if cache.Len() != 0 {
		t.Errorf("cache holds %d entries, want 0", cache.Len())
	}
}

func TestAMatrixIsNotCached(t *testing.T) {
	// Two dispatch rounds share a matrix key only when the same drivers are in
	// the same places, which is when the answer has most likely changed.
	provider := &countingProvider{route: aLiveRoute()}
	caching := routing.NewCachingProvider(provider, routing.NewMemoryCache(nil), 0)
	ctx := context.Background()

	for range 3 {
		if _, err := caching.Matrix(ctx, []routing.Point{lahoreFort},
			[]routing.Point{gulberg}, routing.Options{}); err != nil {
			t.Fatal(err)
		}
	}
	if provider.count() != 3 {
		t.Errorf("provider called %d times, want 3", provider.count())
	}
}

func TestTheCacheIsInvisibleToTheServiceContract(t *testing.T) {
	// Wrapping must not change what Service does with a provider, including
	// falling back when one refuses a mode.
	provider := &countingProvider{route: aLiveRoute()}
	service := routing.NewService(routing.NewCachingProvider(provider, routing.NewMemoryCache(nil), 0))

	route, err := service.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if route.Provider != "counting" {
		t.Errorf("provider = %q, want the wrapped provider's own name", route.Provider)
	}
}

func TestABurstOfIdenticalQuotesIsBilledOnce(t *testing.T) {
	// The cache alone does not help a burst: every caller misses, because none
	// has returned yet to populate the entry. Twenty customers quoting the same
	// trip in the same second is exactly the case a cache exists to prevent,
	// and without single-flight the platform pays twenty times for one answer.
	//
	// Run with -race: a shared cache is read by every in-flight quote.
	release := make(chan struct{})
	provider := &blockingProvider{
		countingProvider: &countingProvider{route: aLiveRoute()},
		release:          release,
	}
	caching := routing.NewCachingProvider(provider, routing.NewMemoryCache(nil), 0)

	const callers = 20
	results := make(chan routing.Route, callers)
	errs := make(chan error, callers)
	for range callers {
		go func() {
			route, err := caching.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
			if err != nil {
				errs <- err
				return
			}
			results <- route
		}()
	}

	// Let them all pile up on the same key before any can answer.
	provider.waitForArrival(t)
	close(release)

	for range callers {
		select {
		case err := <-errs:
			t.Fatal(err)
		case route := <-results:
			if route.DistanceMeters != 12000 {
				t.Errorf("distance = %d, want every caller to get the one answer", route.DistanceMeters)
			}
		}
	}
	if got := provider.count(); got != 1 {
		t.Errorf("provider called %d times for %d simultaneous identical quotes, want 1", got, callers)
	}
}

// blockingProvider holds its answer until released, so a test can be sure every
// caller is waiting on the same key at once rather than arriving one at a time.
type blockingProvider struct {
	*countingProvider
	release chan struct{}
	arrived chan struct{}
	once    sync.Once
}

func (p *blockingProvider) Route(ctx context.Context, origin, destination routing.Point, opts routing.Options) (routing.Route, error) {
	p.once.Do(func() { close(p.arrivedChan()) })
	<-p.release
	return p.countingProvider.Route(ctx, origin, destination, opts)
}

func (p *blockingProvider) arrivedChan() chan struct{} {
	p.countingProvider.mu.Lock()
	defer p.countingProvider.mu.Unlock()
	if p.arrived == nil {
		p.arrived = make(chan struct{})
	}
	return p.arrived
}

func (p *blockingProvider) waitForArrival(t *testing.T) {
	t.Helper()
	select {
	case <-p.arrivedChan():
	case <-time.After(2 * time.Second):
		t.Fatal("no caller reached the provider")
	}
}
