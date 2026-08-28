// Package ratelimit implements the fixed-window limiting documents 20 and 28
// require on authentication.
//
// The limits that matter here are not about load. An unlimited OTP endpoint is
// an SMS bill someone else can run up and a way to enumerate which phone
// numbers hold accounts; an unlimited verify endpoint turns a six-digit code
// into a number an attacker can simply count to.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Decision is the outcome of one check.
type Decision struct {
	Allowed   bool
	Remaining int
	// RetryAfter is how long until the window rolls over. Zero when allowed.
	RetryAfter time.Duration
}

// Rule is a limit: Limit events per Window.
type Rule struct {
	Name   string
	Limit  int
	Window time.Duration
}

// Limiter counts events against a rule.
type Limiter interface {
	Allow(ctx context.Context, rule Rule, key string) (Decision, error)
}

// RedisLimiter counts in Redis so the limit holds across every API instance.
// A per-process limiter would multiply every limit by the instance count,
// which for an SMS-sending endpoint is a real cost.
type RedisLimiter struct {
	client *redis.Client
	prefix string
}

func NewRedisLimiter(client *redis.Client) *RedisLimiter {
	return &RedisLimiter{client: client, prefix: "ratelimit:"}
}

func (l *RedisLimiter) Allow(ctx context.Context, rule Rule, key string) (Decision, error) {
	// Fixed window: the bucket key contains the window index, so it rolls over
	// by changing key and expires on its own. A sliding window would be more
	// even, and needs a sorted set per key — more machinery than an auth
	// endpoint warrants.
	window := time.Now().UTC().UnixNano() / int64(rule.Window)
	bucket := fmt.Sprintf("%s%s:%s:%d", l.prefix, rule.Name, key, window)

	pipe := l.client.TxPipeline()
	count := pipe.Incr(ctx, bucket)
	// Expiry is set on every call rather than only the first: a key that
	// somehow lost its TTL would otherwise leak forever.
	pipe.Expire(ctx, bucket, rule.Window)
	if _, err := pipe.Exec(ctx); err != nil {
		return Decision{}, fmt.Errorf("ratelimit: %w", err)
	}

	used := int(count.Val())
	if used > rule.Limit {
		return Decision{Allowed: false, Remaining: 0, RetryAfter: rule.Window}, nil
	}
	return Decision{Allowed: true, Remaining: rule.Limit - used}, nil
}

// MemoryLimiter is an in-process limiter for tests and single-node
// development. It is not safe to rely on across instances, which is why the
// Redis implementation is the default in cmd/api.
type MemoryLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	now    func() time.Time
}

func NewMemoryLimiter() *MemoryLimiter {
	return &MemoryLimiter{counts: map[string]int{}, now: time.Now}
}

// SetClock is used by tests to advance windows without sleeping.
func (l *MemoryLimiter) SetClock(now func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = now
}

func (l *MemoryLimiter) Allow(_ context.Context, rule Rule, key string) (Decision, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	window := l.now().UTC().UnixNano() / int64(rule.Window)
	bucket := fmt.Sprintf("%s:%s:%d", rule.Name, key, window)
	l.counts[bucket]++

	if l.counts[bucket] > rule.Limit {
		return Decision{Allowed: false, RetryAfter: rule.Window}, nil
	}
	return Decision{Allowed: true, Remaining: rule.Limit - l.counts[bucket]}, nil
}

// ErrLimited is returned by callers that turn a Decision into an error.
var ErrLimited = errors.New("ratelimit: too many requests")
