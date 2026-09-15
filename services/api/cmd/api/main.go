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
	"github.com/sarmadkung/rideme/services/api/internal/credit"
	"github.com/sarmadkung/rideme/services/api/internal/dispatch"
	"github.com/sarmadkung/rideme/services/api/internal/driver"
	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/internal/notify"
	"github.com/sarmadkung/rideme/services/api/internal/places"
	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/internal/realtime"
	"github.com/sarmadkung/rideme/services/api/internal/settings"
	"github.com/sarmadkung/rideme/services/api/internal/settlement"
	"github.com/sarmadkung/rideme/services/api/internal/sweeper"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
	"github.com/sarmadkung/rideme/services/api/internal/zones"
	"github.com/sarmadkung/rideme/services/api/pkg/authn"
	"github.com/sarmadkung/rideme/services/api/pkg/cache"
	"github.com/sarmadkung/rideme/services/api/pkg/config"
	"github.com/sarmadkung/rideme/services/api/pkg/database"
	"github.com/sarmadkung/rideme/services/api/pkg/health"
	"github.com/sarmadkung/rideme/services/api/pkg/messaging"
	otp "github.com/sarmadkung/rideme/services/api/pkg/notify"
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
	if cfg.OTPBypass {
		logger.Warn("AUTH_OTP_BYPASS is enabled: any code verifies a login, real code never checked")
	}

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
	messenger := otp.NewService(logger)
	if cfg.Env.IsProduction() {
		// The development sender logs message bodies, which for an OTP is the
		// credential itself. Refusing to start is better than starting with a
		// provider that prints every login code to the log.
		return fmt.Errorf("no SMS provider is configured for %s: set one before deploying", cfg.Env)
	}
	messenger.Register(otp.ChannelSMS, otp.NewLogSender(logger))
	messenger.Register(otp.ChannelEmail, otp.NewLogSender(logger))

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
		identity.Options{OTPBypass: cfg.OTPBypass},
	)

	// Booking, pricing and routing. The straight-line estimator is always the
	// last resort, so a fare built on a guess is never presented as a measured
	// one; a configured provider simply moves most routes off the guess.
	routingProviders, err := buildRoutingProviders(cfg, redis, logger)
	if err != nil {
		return err
	}
	jobStore := jobs.NewStore(pool.Pool)
	bookingStore := booking.NewStore(pool.Pool)
	// The values the owner decided (BD-01, BD-02, BD-04, BD-11, BD-12) are
	// rows, not constants, so every consumer reads them through one store.
	platformSettings := settings.NewStore(pool.Pool)
	// Zones (document 97): the geographic boundary pricing keys off, alongside
	// the plain city string a quote still carries.
	zoneStore := zones.NewStore(pool.Pool)
	zonesHandler := zones.NewHandler(zones.NewService(zoneStore))
	// One routing service and one tracking store for the whole process:
	// dispatch scores candidates with the same routes a quote was priced from,
	// and reads the same position pool a driver reports into.
	routes := routing.NewService(routingProviders...)
	trackingStore := tracking.NewStore(pool.Pool, redis.Client)
	merchantStore := merchant.NewStore(pool.Pool)
	bookingService := booking.NewService(
		jobStore, bookingStore,
		pricing.NewEngine(nil),
		routes,
		platformSettings,
		zoneStore,
		nil,
	).WithTracking(trackingStore, logger)
	// An order follows the delivery it produced (document 070). Wired after
	// construction because booking is generic over job type and knows nothing
	// about shops — only that some jobs were made on something's behalf.
	bookingService.WithDeliveries(merchant.NewService(merchantStore))
	// The books a finished job is written into.
	//
	// Every piece of `internal/finance` was built, tested and never called:
	// nothing created a payment intent, nothing captured one, and the ledger
	// the earnings surface reads had no writer at all — so a completed trip
	// charged nobody and every driver's earnings were zero because the book
	// was empty. This is the writer.
	//
	// Cash is the only method (the owner's decision, 2026-09-14), so there is
	// no provider to call: the driver is handed the notes and the movement is
	// recorded. WithGoods adds the shop's side, because on a grocery delivery
	// the cash in the driver's hand is mostly somebody else's.
	ledger := finance.NewStore(pool.Pool)
	bookingService.WithSettlement(
		settlement.NewService(ledger, bookingStore, logger).WithGoods(merchantStore))
	providerStore := providers.NewStore(pool.Pool)
	bookingHandler := booking.NewHandler(bookingService, jobStore, providerStore, bookingStore)

	// The driver surface. Availability, position reporting and "what am I
	// holding" — the three things a driver's phone needs that no endpoint
	// offered before.
	// The ledger is the only record of what a driver earned, so the earnings
	// surface reads it directly rather than a total kept beside it.

	// The communication layer (documents 121, 122, 124). The realtime gateway
	// reaches a phone whose app is open and connected; this reaches one in a
	// pocket, which is where a customer's phone is while they wait.
	//
	// Document 121: "Business services emit events. They should not directly
	// call Twilio, Firebase, email providers or other channel vendors." So the
	// provider is an adapter behind a queue, and the only adapter that ships
	// writes to the log — this platform has no push credentials, and an
	// adapter that pretended otherwise would leave a booking believing a
	// customer was told and a customer who was not.
	notifyStore := notify.NewStore(pool.Pool)
	notifyService := notify.NewService(notifyStore, logger)
	notifyHandler := notify.NewHandler(notifyStore)
	bookingService.WithNotifier(notify.NewJobNotifier(notifyService))

	// The realtime gateway. `internal/realtime` was a complete WebSocket hub —
	// channel grammar, authorization, bounded buffers, location coalescing —
	// with no transport and no publishers: nothing outside the package
	// referenced it at all. So every event document 018 lists was a declared
	// constant that was never constructed, and the only way a customer learned
	// their driver had moved was to ask again.
	//
	// Membership is what scopes a job channel to the people on that job;
	// without it the authorizer denies job and merchant channels outright,
	// which fails closed rather than open.
	// The counter an agent stands behind (BD-09). The cap built alongside it
	// could stop a driver working and nothing could start them again: the
	// ledger knew how to record a repayment and no route called it, so a
	// blocked driver was stuck until somebody ran SQL.
	creditHandler := credit.NewHandler(ledger, providerStore)

	hub := realtime.NewHub(realtime.RoleAuthorizer{Membership: jobMembership{jobStore}.can})
	events := realtime.NewPublisher(hub, nil)
	realtimeHandler := realtime.NewHandler(hub, driverIDLookup{providerStore})
	bookingService.WithAnnouncer(events)

	driverHandler := driver.NewHandler(driver.NewService(
		providerStore, trackingStore, jobStore,
		tracking.DefaultLimits(), nil).
		WithLedger(ledger).
		WithRealtime(events, trackingStore))

	// Place search. Built only when a geocoder is configured; the routes are
	// absent otherwise (see places.NewHandler).
	placesHandler := places.NewHandler(buildGeocoder(cfg, logger))

	// The merchant surface (document 072). The order lifecycle and the
	// acceptance deadline were both built and verified in Phase 10; until now
	// nothing served them, so the sweeper below was cancelling orders that no
	// merchant had any way to answer.
	// WithJobs is what turns a ready order into a delivery: document 070's two
	// lifecycles, linked at READY_FOR_PICKUP and nowhere else.
	merchantHandler := merchant.NewHandler(
		merchant.NewService(merchantStore).WithJobs(jobStore).
			// A delivery with no price lock settles at a fare of zero: the
			// driver who carries the shopping earns nothing and the platform
			// earns no commission. Nothing priced a delivery until now —
			// the engine had no GROCERY rule set at all.
			WithPricing(bookingService, logger))

	// The customer's side of the same lifecycle (documents 068, 071): the
	// shops near them, one shop's catalogue, a cart, and a checkout that
	// records where the order is going.
	groceryHandler := merchant.NewCustomerHandler(merchant.NewCustomerService(merchantStore))

	// Answering an offer, and watching the trip it becomes. Both surfaces are
	// new; everything behind them was built and unreachable — the accept and
	// reject routes answered 503 pointing at a dispatch surface that did not
	// exist, and no endpoint had ever served a driver's position to the
	// customer waiting for them.
	dispatchStore := dispatch.NewStore(pool.Pool)
	// A driver's phone learns it has an offer, rather than discovering one by
	// asking. Dispatch gives a driver seconds to answer before the round moves
	// on (BD-04), and polling for something with a countdown attached is why
	// the offer window felt shorter than it is.
	offerHandler := dispatch.NewHandler(dispatchStore, providerStore, trackingStore, logger).
		WithAnnouncer(events, jobStore)
	trackHandler := tracking.NewHandler(trackingStore, driverIDLookup{providerStore})

	server := &http.Server{
		Addr: net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler: newRouter(checker, identity.NewHandler(identityService), bookingHandler,
			driverHandler, zonesHandler, placesHandler, merchantHandler, groceryHandler,
			offerHandler, trackHandler, realtimeHandler, notifyHandler, creditHandler,
			issuer, serviceName, version, logger,
			cfg.CORSAllowedOrigins),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Deadline enforcement (BD-04, BD-12). The values these act on are rows;
	// this is the thing that acts on them. Without it an unanswered grocery
	// order and a job that found no driver both wait forever.
	// The dispatch engine, and the runner that drives it.
	//
	// The engine was built and tested in Phase 8 and never constructed: the
	// runner was created with a nil engine, nothing called Attempt, and no job
	// ever left REQUESTED. A customer's booking reached the database and no
	// driver. Round is the call that closes that gap.
	dispatchEngine := dispatch.NewEngine(dispatchStore, jobStore, providerStore,
		trackingStore, routes, logger, nil).
		WithAnnouncer(events).
		// BD-09: a driver over their credit cap receives no further offers.
		// Enforced in the round as well as at go-online, because a driver
		// crosses the cap on the trip that pushes them over — mid-shift, while
		// already online.
		WithCreditCheck(creditCheck{ledger: ledger, logger: logger})
	dispatchRunner := dispatch.NewRunner(dispatchEngine, jobStore, platformSettings, logger, nil).
		WithOffers(dispatchStore)
	// The send pass. Without it a notification is a row nobody reads: queueing
	// and sending are separate so a provider outage delays delivery rather
	// than failing the booking that caused it.
	notifyWorker := notify.NewWorker(notifyStore, notify.NewLogSender(logger), logger, 0)

	deadlines := sweeper.New(merchantStore, dispatchRunner, logger, 0, nil)
	sweepCtx, stopSweeping := context.WithCancel(context.Background())
	defer stopSweeping()
	go deadlines.Run(sweepCtx)
	go notifyWorker.Run(sweepCtx)

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

// buildRoutingProviders turns MAP_PROVIDER into the provider chain.
//
// An empty chain is legitimate: routing.Service falls back to its straight-line
// estimator and marks every result estimated, which is what a platform without
// a maps contract should show. Config has already refused a selected provider
// with no credential, so a failure here is a real one.
func buildRoutingProviders(cfg *config.Config, redis *cache.Client, logger *slog.Logger) ([]routing.Provider, error) {
	switch cfg.MapProvider {
	case "google":
		provider, err := routing.NewGoogleProvider(cfg.MapsAPIKey, routing.WithGoogleLogger(logger))
		if err != nil {
			return nil, fmt.Errorf("routing provider: %w", err)
		}
		// Every route is billed, and a city quotes the same trips repeatedly
		// (document 104). The cache sits in front of the provider rather than
		// in front of the fallback chain, so a Google route and a straight-line
		// estimate can never share an entry.
		cached := routing.NewCachingProvider(
			provider,
			routing.NewRedisCache(redis.Client, logger),
			routing.DefaultCacheTTL,
		)
		logger.Info("routing provider configured",
			"provider", provider.Name(), "cache_ttl", routing.DefaultCacheTTL.String())
		return []routing.Provider{cached}, nil
	default:
		logger.Warn("no routing provider configured; distances are straight-line estimates",
			"map_provider", cfg.MapProvider)
		return nil, nil
	}
}

// buildGeocoder returns the configured geocoder, or nil.
//
// Nil is a legitimate outcome and not an error: without a maps provider the
// platform has no way to turn text into a place, and the honest response is
// for the search endpoints not to exist rather than to answer badly.
func buildGeocoder(cfg *config.Config, logger *slog.Logger) routing.Geocoder {
	if cfg.MapProvider != "google" {
		return nil
	}
	geocoder, err := routing.NewGoogleGeocoder(cfg.MapsAPIKey, routing.WithGoogleLogger(logger))
	if err != nil {
		// Config has already refused google with no key, so this cannot happen
		// from configuration. Log rather than fail: place search is a
		// convenience, and losing it must not stop the platform booking rides.
		logger.Error("geocoder could not be built; place search is disabled", "error", err.Error())
		return nil
	}
	logger.Info("geocoder configured", "provider", geocoder.Name())
	return geocoder
}

// jobMembership answers whether a subscriber belongs on a job's channel.
//
// The realtime hub takes this as a function because job membership needs a
// database and the gateway must not import a store. The rule is document 102's
// scoping, read as a subscription question: the customer who booked it and the
// driver carrying it, and nobody else. Operations reach a job through the
// admin channel, not by subscribing to somebody's trip.
//
// Any error denies. A membership check that cannot reach the database must not
// resolve to "probably fine" — that is a live position leaking to whoever asked
// at the wrong moment.
type jobMembership struct{ jobs *jobs.Store }

func (m jobMembership) can(sub realtime.Subscriber, ch realtime.Channel) (bool, error) {
	if ch.Kind != realtime.ChannelJob {
		// Merchant channels need a merchant store and their own rule; denying
		// is the honest answer until that exists, rather than a check that
		// looks present and is not.
		return false, nil
	}
	job, err := m.jobs.ByID(context.Background(), ch.ID)
	if err != nil {
		return false, nil
	}
	if sub.UserID != "" && job.RequesterUserID == sub.UserID {
		return true, nil
	}
	return sub.DriverID != "" && job.AssignedDriverID == sub.DriverID, nil
}

// creditCheck asks the ledger whether a driver may carry more platform cash.
//
// It swallows its error deliberately, and this is the one place in the money
// path where that is right: a ledger that cannot be read must not empty the
// candidate pool for the whole city. The platform accepts the risk of one more
// trip, and the failure is logged where somebody will see it.
type creditCheck struct {
	ledger *finance.Store
	logger *slog.Logger
}

func (c creditCheck) MayWork(ctx context.Context, driverID, vehicleType string) bool {
	standing, err := c.ledger.StandingOf(ctx, driverID, vehicleType)
	if err != nil {
		c.logger.Error("could not read a driver's credit standing; allowing the offer",
			slog.String("driver_id", driverID), slog.String("error", err.Error()))
		return true
	}
	return !standing.Blocked
}

// driverIDLookup narrows the provider store to the one question the tracking
// surface asks: is the caller the driver they are watching?
//
// An adapter rather than a wider interface, so tracking does not import
// providers to learn one field.
type driverIDLookup struct{ providers *providers.Store }

func (l driverIDLookup) DriverIDForUser(ctx context.Context, userID string) (string, error) {
	driver, err := l.providers.DriverByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	return driver.ID, nil
}
