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

	"github.com/sarmadkung/rideme/services/api/internal/booking"
	"github.com/sarmadkung/rideme/services/api/internal/dispatch"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/internal/settings"
	"github.com/sarmadkung/rideme/services/api/internal/sweeper"
	"github.com/sarmadkung/rideme/services/api/pkg/authn"
	"github.com/sarmadkung/rideme/services/api/pkg/cache"
	"github.com/sarmadkung/rideme/services/api/pkg/config"
	"github.com/sarmadkung/rideme/services/api/pkg/database"
	"github.com/sarmadkung/rideme/services/api/pkg/health"
	"github.com/sarmadkung/rideme/services/api/pkg/messaging"
	"github.com/sarmadkung/rideme/services/api/pkg/notify"
	"github.com/sarmadkung/rideme/services/api/pkg/observability"
	"github.com/sarmadkung/rideme/services/api/pkg/ratelimit"
	"github.com/sarmadkung/rideme/services/api/pkg/routing"
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

	// Identity (documents 20, 28). The messaging boundary is wired first
	// because authentication cannot ship without it: phone OTP is the initial
	// authentication method and the provider must sit behind an interface.
	messenger := notify.NewService(logger)
	if cfg.Env.IsProduction() {
		// The development sender logs message bodies, which for an OTP is the
		// credential itself. Refusing to start is better than starting with a
		// provider that prints every login code to the log.
		return fmt.Errorf("no SMS provider is configured for %s: set one before deploying", cfg.Env)
	}
	messenger.Register(notify.ChannelSMS, notify.NewLogSender(logger))
	messenger.Register(notify.ChannelEmail, notify.NewLogSender(logger))

	issuer, err := authn.NewIssuer(cfg.JWTSecret)
	if err != nil {
		return fmt.Errorf("token issuer: %w", err)
	}
	identityService := identity.NewService(
		identity.NewStore(pool.Pool),
		issuer,
		messenger,
		ratelimit.NewRedisLimiter(redis.Client),
		logger,
		cfg.JWTSecret,
		identity.Options{},
	)

	// Booking, pricing and routing. The routing service has one provider today
	// — a straight-line estimator that labels every result as estimated — so a
	// fare built on a guess is never presented as a measured one.
	jobStore := jobs.NewStore(pool.Pool)
	bookingStore := booking.NewStore(pool.Pool)
	// The values the owner decided (BD-01, BD-02, BD-04, BD-11, BD-12) are
	// rows, not constants, so every consumer reads them through one store.
	platformSettings := settings.NewStore(pool.Pool)
	bookingService := booking.NewService(
		jobStore, bookingStore,
		pricing.NewEngine(nil),
		routing.NewService(),
		platformSettings,
		nil,
	)
	bookingHandler := booking.NewHandler(bookingService, jobStore, providers.NewStore(pool.Pool))

	server := &http.Server{
		Addr: net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler: newRouter(checker, identity.NewHandler(identityService), bookingHandler,
			issuer, serviceName, version, logger),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Deadline enforcement (BD-04, BD-12). The values these act on are rows;
	// this is the thing that acts on them. Without it an unanswered grocery
	// order and a job that found no driver both wait forever.
	dispatchRunner := dispatch.NewRunner(nil, jobStore, platformSettings, logger, nil)
	deadlines := sweeper.New(merchant.NewStore(pool.Pool), dispatchRunner, logger, 0, nil)
	sweepCtx, stopSweeping := context.WithCancel(context.Background())
	defer stopSweeping()
	go deadlines.Run(sweepCtx)

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

	// Stop sweeping before draining requests: a pass that starts during
	// shutdown would hold a transaction open against a closing pool.
	stopSweeping()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	logger.Info("stopped cleanly")
	return nil
}
