package dispatch

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/settings"
)

// Runner is BD-04, resolved on 2026-08-28: dispatch retries with a widening
// radius for a bounded time, and a job that still finds nobody ends as EXPIRED
// with a NO_SUPPLY reason. The customer is not charged.
//
// The engine deliberately does not expire jobs itself — it runs one attempt
// and reports what happened. Deciding that a search is over is a policy
// question, and keeping it here means the rule lives in one place instead of
// being spread through the pipeline.
type Runner struct {
	engine   *Engine
	jobs     *jobs.Store
	settings *settings.Store
	offers   *Store
	logger   *slog.Logger
	now      func() time.Time
}

func NewRunner(engine *Engine, jobStore *jobs.Store, platformSettings *settings.Store,
	logger *slog.Logger, now func() time.Time) *Runner {
	if now == nil {
		now = time.Now
	}
	return &Runner{engine: engine, jobs: jobStore, settings: platformSettings, logger: logger, now: now}
}

// WithOffers attaches the dispatch store, so a round can release offers whose
// TTL has passed before deciding which jobs still need one.
//
// Optional, and nil is a legitimate state: a Runner built only to expire stale
// searches — which is all the sweeper had before there was anything driving
// dispatch — needs no offer sweep.
func (r *Runner) WithOffers(store *Store) *Runner {
	r.offers = store
	return r
}

// SearchDeadline reads how long a job may search before it expires.
func (r *Runner) SearchDeadline(ctx context.Context) (time.Duration, error) {
	return r.settings.Duration(ctx, settings.KeyDispatchSearchDeadlineSeconds)
}

// Attempt runs one dispatch round and expires the job if the search is over.
//
// Two conditions end a search, and both are checked because either alone
// leaves a hole: attempts alone would let a job with slow rounds run past its
// deadline, and the deadline alone would let a job that exhausted its rings in
// two seconds sit idle until the clock caught up.
func (r *Runner) Attempt(ctx context.Context, jobID string) (Result, error) {
	job, err := r.jobs.ByID(ctx, jobID)
	if err != nil {
		return Result{}, err
	}
	if job.Status != jobs.StatusSearching {
		return Result{}, nil
	}

	deadline, err := r.SearchDeadline(ctx)
	if err != nil {
		return Result{}, err
	}
	if r.now().Sub(job.UpdatedAt) >= deadline {
		return r.expire(ctx, jobID, "search deadline reached")
	}
	if r.engine == nil {
		// A Runner built for expiry alone. The deadline above still applies —
		// a search nothing is driving must still end — but there is nothing
		// here to offer the job to.
		return Result{}, nil
	}

	result, err := r.engine.Dispatch(ctx, jobID)
	if errors.Is(err, ErrNoSupply) {
		// Every configured ring has been tried. Nothing widens further.
		return r.expire(ctx, jobID, "all dispatch rings exhausted")
	}
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// Sweep expires every job that has been searching past its deadline.
//
// This is the safety net for a dispatch loop that stopped running — a crashed
// worker, a restarted process. Without it those jobs stay SEARCHING forever,
// and a customer watches a spinner for a search nothing is driving.
func (r *Runner) Sweep(ctx context.Context, limit int) (int, error) {
	deadline, err := r.SearchDeadline(ctx)
	if err != nil {
		return 0, err
	}
	stale, err := r.jobs.SearchingSince(ctx, r.now().Add(-deadline), limit)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, jobID := range stale {
		if _, err := r.expire(ctx, jobID, "search deadline reached"); err != nil {
			// One job failing to expire must not stop the rest of the sweep;
			// the most likely cause is a driver accepting it mid-pass, which
			// is the correct outcome rather than an error worth aborting on.
			r.logger.Warn("could not expire a stale search",
				slog.String("job_id", jobID), slog.String("error", err.Error()))
			continue
		}
		expired++
	}
	return expired, nil
}

func (r *Runner) expire(ctx context.Context, jobID, why string) (Result, error) {
	if _, err := r.jobs.ExpireSearch(ctx, jobID, jobs.ReasonNoSupply); err != nil {
		if errors.Is(err, jobs.ErrStaleTransition) {
			// A driver accepted while we were deciding to give up. Their
			// acceptance stands.
			return Result{Outcome: OutcomeOffered}, nil
		}
		return Result{}, err
	}
	r.logger.Info("job expired with no supply",
		slog.String("job_id", jobID), slog.String("why", why))
	return Result{Outcome: OutcomeExhausted}, nil
}

// RoundResult is what one dispatch pass did.
type RoundResult struct {
	// Released is offers whose TTL passed, freeing their job for the next ring.
	Released int64
	// Started is jobs that entered dispatch for the first time.
	Started int
	// Offered is jobs that came out of this pass held by a driver.
	Offered int
	// Considered is how many jobs the pass looked at.
	Considered int
}

// Round drives every job that is waiting for dispatch.
//
// This is the wire that was missing. Booking created a job as REQUESTED,
// StartSearching had no callers, the engine was never constructed, and
// Runner.Attempt no-ops on anything that is not already SEARCHING — so a
// customer's job was written to the database and offered to nobody, for as
// long as the platform had existed. Phase 7 and Phase 8 were both verified;
// between them there was no call.
//
// The order matters. Expired offers are released first, because a job whose
// offer just timed out is exactly a job that needs the next ring, and doing it
// the other way round makes that job wait a whole extra pass.
//
// One job failing does not stop the pass. The likely causes are a driver
// accepting mid-round and a job with no pickup stop, and neither is a reason
// to leave every other waiting customer unserved.
func (r *Runner) Round(ctx context.Context, limit int) (RoundResult, error) {
	var result RoundResult
	if r.engine == nil {
		// Nothing to dispatch with. Moving jobs into SEARCHING anyway would
		// leave them there for the deadline sweep to expire, which is worse
		// than leaving them REQUESTED and visibly undispatched.
		return result, nil
	}

	if r.offers != nil {
		released, err := r.offers.SweepExpired(ctx, r.now())
		if err != nil {
			// Not fatal: the jobs whose offers are still held are simply not
			// due yet, and the rest of the pass is still worth running.
			r.logger.Warn("could not release expired offers", slog.String("error", err.Error()))
		}
		result.Released = released
	}

	waiting, err := r.jobs.NeedingDispatch(ctx, r.now(), limit)
	if err != nil {
		return result, err
	}
	result.Considered = len(waiting)

	for _, jobID := range waiting {
		started, err := r.start(ctx, jobID)
		if err != nil {
			r.logger.Warn("could not start dispatch for a job",
				slog.String("job_id", jobID), slog.String("error", err.Error()))
			continue
		}
		if started {
			result.Started++
		}

		attempt, err := r.Attempt(ctx, jobID)
		if err != nil {
			r.logger.Warn("dispatch attempt failed",
				slog.String("job_id", jobID), slog.String("error", err.Error()))
			continue
		}
		if attempt.Outcome == OutcomeOffered {
			result.Offered++
		}
	}
	return result, nil
}

// start moves a job into SEARCHING if it is not there yet.
//
// Compare-and-set on REQUESTED, so a job cancelled between the query and this
// write is left alone rather than dragged back into a search: the transition
// matches no rows and the job is skipped.
func (r *Runner) start(ctx context.Context, jobID string) (bool, error) {
	job, err := r.jobs.ByID(ctx, jobID)
	if err != nil {
		return false, err
	}
	if job.Status != jobs.StatusRequested {
		return false, nil
	}
	if _, err := r.jobs.Transition(ctx, jobID, jobs.StatusRequested, jobs.StatusSearching,
		jobs.Actor{Type: jobs.ActorSystem}, map[string]any{"reason": "entered dispatch"}); err != nil {
		return false, err
	}
	return true, nil
}
