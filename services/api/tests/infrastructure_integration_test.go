//go:build integration

// Package tests holds tests that require the local infrastructure from
// infra/docker/docker-compose.yml. They are excluded from `go test ./...` by the
// build tag above.
//
//	go test -tags=integration ./tests/...
package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/cache"
	"github.com/sarmadkung/rideme/services/api/pkg/database"
	"github.com/sarmadkung/rideme/services/api/pkg/messaging"
)

func env(t *testing.T, key, fallback string) string {
	t.Helper()
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func TestPostgresIsReachableAndHasPostGIS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, database.Options{
		URL: env(t, "DATABASE_URL",
			"postgres://logistics:logistics@localhost:5432/logistics_dev?sslmode=disable"),
	})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	installed, err := pool.HasPostGIS(ctx)
	if err != nil {
		t.Fatalf("check postgis: %v", err)
	}
	if !installed {
		t.Fatal("PostGIS is not installed — run `make migrate-up`")
	}
}

func TestRedisIsReachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := cache.Connect(ctx, env(t, "REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("ping redis: %v", err)
	}
}

func TestNATSIsReachable(t *testing.T) {
	conn, err := messaging.Connect(env(t, "NATS_URL", "nats://localhost:4222"), "integration-test")
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	defer conn.Close()

	if !conn.Healthy() {
		t.Fatal("nats connection is not healthy")
	}
}
