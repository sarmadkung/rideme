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
