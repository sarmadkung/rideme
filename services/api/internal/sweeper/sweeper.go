// Package sweeper runs the periodic work that turns stored deadlines into
// actions.
//
// Three of the owner's 2026-08-28 decisions set a clock: a merchant has ten
// minutes to accept an order (BD-12), a job may search for ninety seconds
// (BD-04), and both must then end on their own. A deadline nothing acts on is
// a timestamp, not a rule — an order sent to a closed store waits forever, and
// a customer watches a spinner for a search no worker is driving.
package sweeper

import (
	"context"
	"log/slog"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/dispatch"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
)

// Sweeper runs the deadline passes on an interval.
type Sweeper struct {
	orders   *merchant.Store
	dispatch *dispatch.Runner
	logger   *slog.Logger
	interval time.Duration
	// batch bounds one pass, so a backlog is worked through over several ticks
	// rather than in one long transaction holding locks.
	batch int
	now   func() time.Time
}

func New(orders *merchant.Store, runner *dispatch.Runner, logger *slog.Logger,
	interval time.Duration, now func() time.Time) *Sweeper {
	if interval <= 0 {
		// Well under the shortest deadline it enforces, so an order expires
		// within seconds of its ten minutes rather than minutes after.
		interval = 15 * time.Second
	}
	if now == nil {
		now = time.Now
	}
	return &Sweeper{orders: orders, dispatch: runner, logger: logger,
		interval: interval, batch: 200, now: now}
}

// Run sweeps until the context is cancelled.
//
// A failing pass is logged and the loop continues. The usual cause is a row
// that moved underneath the sweep — a merchant accepting as it expires, a
// driver accepting as a search gives up — which is the correct outcome rather
// than an error worth stopping for.
func (s *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("sweeper stopped")
			return
		case <-ticker.C:
			s.Once(ctx)
		}
	}
}

// Once runs a single pass. Exported so a test can drive it without a clock.
func (s *Sweeper) Once(ctx context.Context) {
	if s.orders != nil {
		expired, err := s.orders.ExpireOverdue(ctx, s.now(), s.batch)
		if err != nil {
			s.logger.Error("order acceptance sweep failed", slog.String("error", err.Error()))
		} else if len(expired) > 0 {
			s.logger.Info("orders expired unanswered", slog.Int("count", len(expired)))
		}
	}
	if s.dispatch != nil {
		expired, err := s.dispatch.Sweep(ctx, s.batch)
		if err != nil {
			s.logger.Error("dispatch search sweep failed", slog.String("error", err.Error()))
		} else if expired > 0 {
			s.logger.Info("searches expired with no supply", slog.Int("count", expired))
		}
	}
}
