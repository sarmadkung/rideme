// Package database owns the PostgreSQL connection pool. PostGIS is required
// (document 12); the platform is location-native and a database without it is
// misconfigured, not merely reduced.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct {
	*pgxpool.Pool
}

type Options struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

func (o Options) withDefaults() Options {
	if o.MaxConns == 0 {
		o.MaxConns = 20
	}
	if o.MinConns == 0 {
		o.MinConns = 2
	}
	if o.MaxConnLifetime == 0 {
		o.MaxConnLifetime = time.Hour
	}
	if o.MaxConnIdleTime == 0 {
		o.MaxConnIdleTime = 30 * time.Minute
	}
	return o
}

// Connect opens the pool and verifies it. Failing here is deliberate: an API
// that starts without a database only defers the outage to the first request.
func Connect(ctx context.Context, opts Options) (*Pool, error) {
	opts = opts.withDefaults()

	cfg, err := pgxpool.ParseConfig(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = opts.MaxConns
	cfg.MinConns = opts.MinConns
	cfg.MaxConnLifetime = opts.MaxConnLifetime
	cfg.MaxConnIdleTime = opts.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Pool{Pool: pool}, nil
}

// Ping is the health probe.
func (p *Pool) Ping(ctx context.Context) error {
	return p.Pool.Ping(ctx)
}

// HasPostGIS reports whether the extension is installed. Checked at startup so
// a missing extension surfaces as a clear message, not as a query failure
// months later.
func (p *Pool) HasPostGIS(ctx context.Context) (bool, error) {
	var installed bool
	err := p.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'postgis')`,
	).Scan(&installed)
	if err != nil {
		return false, fmt.Errorf("check postgis extension: %w", err)
	}
	return installed, nil
}

func (p *Pool) Close() { p.Pool.Close() }
