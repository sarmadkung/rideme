//go:build integration

package tests

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/dispatch"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/internal/settings"
	"github.com/sarmadkung/rideme/services/api/internal/sweeper"
)

// BD-04, resolved on 2026-08-28: dispatch retries with a widening radius for a
// bounded time, and a job that still finds nobody ends as EXPIRED with a
// NO_SUPPLY reason. Nothing is charged.

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestTheSearchDeadlineIsNinetySeconds(t *testing.T) {
	h := newDispatchHarness(t)
	runner := dispatch.NewRunner(nil, h.jobs, settings.NewStore(h.pool), quietLogger(), nil)

	deadline, err := runner.SearchDeadline(context.Background())
	if err != nil {
		t.Fatalf("SearchDeadline: %v", err)
	}
	if deadline != 90*time.Second {
		t.Fatalf("search deadline = %v, want the decided 90s", deadline)
	}
}

func TestAJobThatFindsNobodyExpiresWithNoSupply(t *testing.T) {
	h := newDispatchHarness(t)
	ctx := context.Background()
	job := h.aSearchingJob(t)

	expired, err := h.jobs.ExpireSearch(ctx, job.ID, jobs.ReasonNoSupply)
	if err != nil {
		t.Fatalf("ExpireSearch: %v", err)
	}
	if expired.Status != jobs.StatusExpired {
		t.Fatalf("status = %s, want EXPIRED", expired.Status)
	}

	var reason string
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(failure_reason, '') FROM jobs WHERE id = $1`, job.ID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "NO_SUPPLY" {
		t.Fatalf("failure reason = %q, want NO_SUPPLY", reason)
	}

	// The customer must be able to find out why, which means the transition is
	// in the history rather than only in the job row.
	history, err := h.jobs.History(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := history[len(history)-1]
	if last.To != jobs.StatusExpired {
		t.Fatalf("last recorded transition is to %s", last.To)
	}
}

func TestNothingIsChargedForAJobThatFoundNoDriver(t *testing.T) {
	// The point of ending as EXPIRED rather than CANCELLED: nobody was
	// dispatched, so there is no cancellation and no fee.
	h := newDispatchHarness(t)
	ctx := context.Background()
	job := h.aSearchingJob(t)

	if _, err := h.jobs.ExpireSearch(ctx, job.ID, jobs.ReasonNoSupply); err != nil {
		t.Fatal(err)
	}

	var cancellations int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM job_cancellations WHERE job_id = $1`, job.ID).Scan(&cancellations); err != nil {
		t.Fatal(err)
	}
	if cancellations != 0 {
		t.Fatalf("expiring a search recorded %d cancellations", cancellations)
	}
}

func TestADriverAcceptingBeatsTheExpiryRunningAtTheSameMoment(t *testing.T) {
	// The race that decides whether a customer is told "no drivers" about a
	// job that just found one. Compare-and-set on SEARCHING settles it.
	h := newDispatchHarness(t)
	ctx := context.Background()
	job := h.aSearchingJob(t)
	driver := h.anApprovedDriver(t)

	var wg sync.WaitGroup
	var assigned, expired bool
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, err := h.store.Reserve(ctx, job.ID, driver, "", "", 0.9, time.Minute)
		if err != nil {
			return
		}
		_, err = h.jobs.Transition(ctx, job.ID, jobs.StatusSearching, jobs.StatusAssigned,
			jobs.Actor{Type: jobs.ActorSystem}, nil)
		assigned = err == nil
	}()
	go func() {
		defer wg.Done()
		_, err := h.jobs.ExpireSearch(ctx, job.ID, jobs.ReasonNoSupply)
		expired = err == nil
	}()
	wg.Wait()

	if assigned && expired {
		t.Fatal("a job was assigned to a driver and expired as having no supply")
	}

	final, err := h.jobs.ByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assigned && final.Status != jobs.StatusAssigned {
		t.Fatalf("the driver was assigned but the job is %s", final.Status)
	}
	if expired && final.Status != jobs.StatusExpired {
		t.Fatalf("the job expired but reads %s", final.Status)
	}
}

func TestTheSweepExpiresOnlyJobsPastTheirDeadline(t *testing.T) {
	// The safety net for a dispatch loop that stopped running. A fresh search
	// must survive it; a stale one must not.
	h := newDispatchHarness(t)
	ctx := context.Background()
	fresh := h.aSearchingJob(t)
	stale := h.aSearchingJob(t)

	// Age one job past the 90-second deadline.
	if _, err := h.pool.Exec(ctx,
		`UPDATE jobs SET updated_at = now() - interval '10 minutes' WHERE id = $1`, stale.ID); err != nil {
		t.Fatal(err)
	}

	runner := dispatch.NewRunner(nil, h.jobs, settings.NewStore(h.pool), quietLogger(), nil)
	if _, err := runner.Sweep(ctx, 500); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	staleAfter, err := h.jobs.ByID(ctx, stale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if staleAfter.Status != jobs.StatusExpired {
		t.Fatalf("a search stalled for ten minutes is still %s", staleAfter.Status)
	}

	freshAfter, err := h.jobs.ByID(ctx, fresh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if freshAfter.Status != jobs.StatusSearching {
		t.Fatalf("a search that just started was expired: %s", freshAfter.Status)
	}
}

func TestTheSweeperEnforcesBothDeadlinesInOnePass(t *testing.T) {
	// The wiring, not the rules: the sweeper the server runs must actually
	// drive both BD-04's search deadline and BD-12's acceptance timeout. Each
	// is tested on its own elsewhere; this asserts they are both connected.
	dh := newDispatchHarness(t)
	mh := newMerchantHarness(t)
	ctx := context.Background()

	stale := dh.aSearchingJob(t)
	if _, err := dh.pool.Exec(ctx,
		`UPDATE jobs SET updated_at = now() - interval '10 minutes' WHERE id = $1`, stale.ID); err != nil {
		t.Fatal(err)
	}

	shop := mh.aShop(t)
	cart, err := mh.store.OpenCart(ctx, shop.merchantID, shop.storeID, mh.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mh.store.AddItem(ctx, cart.ID, shop.productID, "", 1, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}
	if _, err := mh.store.Place(ctx, cart.ID, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	runner := dispatch.NewRunner(nil, dh.jobs, settings.NewStore(dh.pool), quietLogger(), nil)
	sweeper.New(mh.store, runner, quietLogger(), 0, nil).Once(ctx)

	job, err := dh.jobs.ByID(ctx, stale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusExpired {
		t.Fatalf("the stalled search is still %s after a sweep", job.Status)
	}

	order, err := mh.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != merchant.StatusCancelled {
		t.Fatalf("the unanswered order is still %s after a sweep", order.Status)
	}
}
