package routing

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache is the Cache the API runs on.
//
// Redis rather than process memory because the platform runs more than one API
// instance: a per-process cache would hold one copy each and divide the hit
// rate — and therefore the saving — by the instance count.
//
// Every failure here is swallowed and logged. A cache that can return an error
// makes routing fail for a reason that has nothing to do with routing; the
// worst a broken cache may do is cost money.
type RedisCache struct {
	client *redis.Client
	logger *slog.Logger
}

func NewRedisCache(client *redis.Client, logger *slog.Logger) *RedisCache {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &RedisCache{client: client, logger: logger}
}

func (c *RedisCache) Get(ctx context.Context, key string) (Route, bool) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		// A miss is the normal path, not a fault worth logging on every quote.
		if err != redis.Nil {
			c.logger.WarnContext(ctx, "route cache read failed", "error", err.Error())
		}
		return Route{}, false
	}
	var route Route
	if err := json.Unmarshal(raw, &route); err != nil {
		// A value written by an older shape. Treating it as a miss re-fetches
		// and overwrites it, which is cheaper than a migration.
		c.logger.WarnContext(ctx, "route cache holds an unreadable entry", "error", err.Error())
		return Route{}, false
	}
	return route, true
}

func (c *RedisCache) Set(ctx context.Context, key string, route Route, ttl time.Duration) {
	raw, err := json.Marshal(route)
	if err != nil {
		c.logger.WarnContext(ctx, "route could not be encoded for the cache", "error", err.Error())
		return
	}
	if err := c.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		c.logger.WarnContext(ctx, "route cache write failed", "error", err.Error())
	}
}

var _ Cache = (*RedisCache)(nil)
