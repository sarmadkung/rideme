// Command api is the platform's HTTP entry point.
//
// Phase 1 boots the process, proves every infrastructure dependency, serves
// health, and shuts down cleanly. It implements no domain behaviour.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/cache"
	"github.com/sarmadkung/rideme/services/api/pkg/config"
	"github.com/sarmadkung/rideme/services/api/pkg/database"
	"github.com/sarmadkung/rideme/services/api/pkg/health"
	"github.com/sarmadkung/rideme/services/api/pkg/messaging"
	"github.com/sarmadkung/rideme/services/api/pkg/observability"
)

const serviceName = "api"

// version is stamped at build time: -ldflags "-X main.version=$(git rev-parse --short HEAD)"
var version = "dev"

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet, so failure goes to stderr directly.
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Development convenience only. A missing file is not an error: staging and
	// production supply configuration through the environment.
	config.LoadDotEnv()

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	logger := observability.NewLogger(os.Stdout, cfg.LogLevel, serviceName, version)
	slog.SetDefault(logger)

	logger.Info("starting", slog.String("env", string(cfg.Env)), slog.Int("port", cfg.Port))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	startupCtx, cancelStartup := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancelStartup()

	// Every dependency is contacted before the listener opens. An API that
	// accepts traffic it cannot serve is worse than one that refuses to start.
	pool, err := database.Connect(startupCtx, database.Options{URL: cfg.DatabaseURL})
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	postgis, err := pool.HasPostGIS(startupCtx)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	if !postgis {
		return errors.New("postgres: PostGIS is not installed — run `make migrate-up` " +
			"(the platform's schema is geospatial and cannot work without it)")
	}

	redis, err := cache.Connect(startupCtx, cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	defer func() { _ = redis.Close() }()

	nats, err := messaging.Connect(cfg.NATSURL, serviceName)
	if err != nil {
		return fmt.Errorf("nats: %w", err)
	}
	defer nats.Close()

	logger.Info("dependencies ready",
		slog.Bool("postgis", postgis),
		slog.String("nats_server", nats.ConnectedUrl()),
	)

	checker := health.NewChecker(serviceName, version, []health.Check{
		{Name: "postgres", Critical: true, Probe: pool.Ping},
		{Name: "redis", Critical: true, Probe: redis.Ping},
		{Name: "nats", Critical: true, Probe: func(context.Context) error {
			if !nats.Healthy() {
				return errors.New("not connected")
			}
			return nil
		}},
	})

	server := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler:           newRouter(checker, serviceName, version, logger),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	// Graceful shutdown: stop accepting, let in-flight requests finish. Once
	// jobs and payments exist, killing a request mid-transaction is a real cost.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	logger.Info("stopped cleanly")
	return nil
}
