package routing

import (
	"context"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Cache stores routes between calls.
//
// A miss is not an error: the caller routes normally and the trip is only
// slower, never wrong. Implementations must therefore swallow their own
// failures rather than surface them — a Redis outage should cost money, not
// bookings.
type Cache interface {
	Get(ctx context.Context, key string) (Route, bool)
	Set(ctx context.Context, key string, route Route, ttl time.Duration)
}

// CachingProvider serves repeat routes from a Cache instead of the provider.
//
// Document 104 lists route requests among the platform's principal map costs
// and asks directly to "avoid duplicate route requests" and "reuse route
// estimates". Two customers quoting the same trip a minute apart is the common
// case in a city, and the second one does not need to be billed.
//
// It wraps a Provider rather than living inside Service so that the cache sits
// in front of exactly one provider and its results are attributed to it. A
// cache in front of the fallback chain would key a Google route and a
// straight-line estimate to the same entry.
type CachingProvider struct {
	provider Provider
	cache    Cache
	ttl      time.Duration
	// inflight collapses concurrent requests for the same key.
	//
	// The cache alone does not help a burst: twenty customers quoting the same
	// trip in the same second all miss, because none of them has returned yet
	// to populate the entry. Without this the platform pays twenty times for
	// one answer, which is the case a cache exists to prevent.
	inflight singleflight.Group
}

// DefaultCacheTTL is how long a route may be reused.
//
// Short, and deliberately so. The road distance between two points barely
// changes, but TrafficDurationSeconds is a statement about right now — an hour
// old it is not a stale number, it is a wrong one. Five minutes keeps the
// traffic reading defensible while still collapsing the burst of quotes a
// popular route attracts.
const DefaultCacheTTL = 5 * time.Minute

// cachePrecision is the number of decimal places a coordinate keeps in a key.
//
// Four places is about 11 metres, which is finer than a phone's GPS fix. This
// is deliberately not a coarse grid: rounding harder would raise the hit rate
// by answering with a route that starts somewhere the customer is not, and a
// fare must be quoted for the trip that was asked about.
const cachePrecision = 4

// NewCachingProvider wraps a provider. A zero ttl means DefaultCacheTTL.
func NewCachingProvider(provider Provider, cache Cache, ttl time.Duration) *CachingProvider {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &CachingProvider{provider: provider, cache: cache, ttl: ttl}
}

func (p *CachingProvider) Name() string    { return p.provider.Name() }
func (p *CachingProvider) Version() string { return p.provider.Version() }

// RouteCacheKey identifies a route request.
//
// The mode is part of the key because a truck and a motorcycle asking about the
// same two points are asking different questions, and the provider version is
// part of it because an upgraded provider that routes differently must not
// serve the previous one's answers.
func RouteCacheKey(provider, version string, origin, destination Point, mode Mode) string {
	round := func(v float64) string {
		return strconv.FormatFloat(
			math.Round(v*math.Pow10(cachePrecision))/math.Pow10(cachePrecision),
			'f', cachePrecision, 64)
	}
	if mode == "" {
		mode = ModeDriving
	}
	return strings.Join([]string{
		"route", provider, version,
		round(origin.Lat), round(origin.Lon),
		round(destination.Lat), round(destination.Lon),
		string(mode),
	}, ":")
}

func (p *CachingProvider) Route(ctx context.Context, origin, destination Point, opts Options) (Route, error) {
	// A scheduled booking asks about a departure time that is not now, so its
	// answer is not the one a cache holding "now" results should return, and
	// storing it would poison them.
	if !opts.DepartAt.IsZero() {
		return p.provider.Route(ctx, origin, destination, opts)
	}

	key := RouteCacheKey(p.provider.Name(), p.provider.Version(), origin, destination, opts.Mode)
	if route, ok := p.cache.Get(ctx, key); ok {
		// Document 96: "Never present a fallback as exact." A reused route is
		// not a fresh measurement, and the caller is told which it has.
		route.Confidence = ConfidenceCached
		return route, nil
	}

	fetched, err, _ := p.inflight.Do(key, func() (any, error) {
		route, err := p.provider.Route(ctx, origin, destination, opts)
		if err != nil {
			// Errors are not cached. A rejected key or a rate limit is a
			// condition that clears; remembering it would extend an outage
			// past its end.
			return Route{}, err
		}
		p.cache.Set(ctx, key, route, p.ttl)
		return route, nil
	})
	if err != nil {
		return Route{}, err
	}
	// The single winner's result is shared by every caller that waited on it.
	// They asked the same question, so they get the same answer — but it was
	// measured once.
	return fetched.(Route), nil
}

// Matrix passes straight through.
//
// A matrix key is the whole grid, so two dispatch rounds a second apart share
// an entry only when the same drivers are in the same places — which is the one
// case where the answer has most likely changed. Document 104's advice for
// matrices is to batch them, not to cache them.
func (p *CachingProvider) Matrix(ctx context.Context, origins, destinations []Point, opts Options) (Matrix, error) {
	return p.provider.Matrix(ctx, origins, destinations, opts)
}

// --- in-memory cache ---------------------------------------------------------

// MemoryCache is a process-local Cache.
//
// Useful in tests and in a single-instance deployment. It is not a substitute
// for Redis across several API instances, where each process would hold its own
// copy and the hit rate would fall by the number of instances.
type MemoryCache struct {
	mu      sync.Mutex
	entries map[string]memoryEntry
	now     func() time.Time
}

type memoryEntry struct {
	route     Route
	expiresAt time.Time
}

// NewMemoryCache builds a cache. A nil clock means time.Now.
func NewMemoryCache(now func() time.Time) *MemoryCache {
	if now == nil {
		now = time.Now
	}
	return &MemoryCache{entries: map[string]memoryEntry{}, now: now}
}

func (c *MemoryCache) Get(_ context.Context, key string) (Route, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return Route{}, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return Route{}, false
	}
	return entry.route, true
}

func (c *MemoryCache) Set(_ context.Context, key string, route Route, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = memoryEntry{route: route, expiresAt: c.now().Add(ttl)}
}

// Len reports how many entries are held, expired ones included.
func (c *MemoryCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

var _ Cache = (*MemoryCache)(nil)
