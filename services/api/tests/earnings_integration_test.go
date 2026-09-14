//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// post writes a driver earning and returns what the driver keeps.
func (h *financeHarness) anEarning(t *testing.T, driverID, jobID string, gross, commission int64) money.Amount {
	t.Helper()
	transaction, err := finance.DriverEarning(
		money.MustNew(gross, money.PKR),
		money.MustNew(commission, money.PKR),
		driverID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Post(context.Background(), transaction); err != nil {
		t.Fatal(err)
	}
	return money.MustNew(gross-commission, money.PKR)
}

func TestEarningsAreWhatTheDriverKeepsNotWhatTheCustomerPaid(t *testing.T) {
	// BD-05 is a flat 20% commission. A driver shown the gross would query
	// every payout they ever received.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customer := h.aUser(t)
	driver := h.aUser(t)
	job := h.aJob(t, customer)

	h.anEarning(t, driver, job, 50000, 10000) // PKR 500 gross, PKR 100 commission

	earnings, err := h.store.EarningsBetween(ctx, driver,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if earnings.Net.Minor != 40000 {
		t.Errorf("net = %d, want 40000 (PKR 500 less 20%%)", earnings.Net.Minor)
	}
	if earnings.Trips != 1 {
		t.Errorf("trips = %d, want 1", earnings.Trips)
	}
}

func TestEarningsAreASumNotASingleTrip(t *testing.T) {
	h := newFinanceHarness(t)
	ctx := context.Background()
	customer := h.aUser(t)
	driver := h.aUser(t)

	for _, gross := range []int64{30000, 45000, 12000} {
		h.anEarning(t, driver, h.aJob(t, customer), gross, gross/5)
	}

	earnings, err := h.store.EarningsBetween(ctx, driver,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// (30000 + 45000 + 12000) less 20% = 69600.
	if earnings.Net.Minor != 69600 {
		t.Errorf("net = %d, want 69600", earnings.Net.Minor)
	}
	if earnings.Trips != 3 {
		t.Errorf("trips = %d, want 3", earnings.Trips)
	}
}

func TestOneDriverNeverSeesAnothersEarnings(t *testing.T) {
	h := newFinanceHarness(t)
	ctx := context.Background()
	customer := h.aUser(t)
	mine, theirs := h.aUser(t), h.aUser(t)

	h.anEarning(t, mine, h.aJob(t, customer), 50000, 10000)
	h.anEarning(t, theirs, h.aJob(t, customer), 90000, 18000)

	earnings, err := h.store.EarningsBetween(ctx, mine,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if earnings.Net.Minor != 40000 {
		t.Errorf("net = %d, want only this driver's 40000", earnings.Net.Minor)
	}
}

func TestAReversedEarningIsSubtractedWithoutBeingSpecialCased(t *testing.T) {
	// A reversal writes an opposing entry, so the sum already reflects it.
	// Anything that remembered and excluded reversals separately would be a
	// second rule to keep in step with document 53.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customer := h.aUser(t)
	driver := h.aUser(t)
	job := h.aJob(t, customer)

	original, err := finance.DriverEarning(
		money.MustNew(50000, money.PKR), money.MustNew(10000, money.PKR), driver, job)
	if err != nil {
		t.Fatal(err)
	}
	posted, err := h.store.Post(ctx, original)
	if err != nil {
		t.Fatal(err)
	}

	reversal, err := finance.Reverse(posted, "trip disputed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Post(ctx, reversal); err != nil {
		t.Fatal(err)
	}

	earnings, err := h.store.EarningsBetween(ctx, driver,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if earnings.Net.Minor != 0 {
		t.Errorf("net = %d, want 0 after the reversal", earnings.Net.Minor)
	}
}

func TestAWindowExcludesWhatFallsOutsideIt(t *testing.T) {
	// `from` inclusive and `to` exclusive, so consecutive days count nothing
	// twice.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customer := h.aUser(t)
	driver := h.aUser(t)
	h.anEarning(t, driver, h.aJob(t, customer), 50000, 10000)

	before := time.Now().Add(-2 * time.Hour)
	earnings, err := h.store.EarningsBetween(ctx, driver, before, before.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if earnings.Net.Minor != 0 {
		t.Errorf("net = %d, want 0 for a window the earning is outside",
			earnings.Net.Minor)
	}
	if earnings.Trips != 0 {
		t.Errorf("trips = %d, want 0", earnings.Trips)
	}
}

func TestADriverWithNoEarningsIsZeroNotAnError(t *testing.T) {
	// A new driver opening the screen must see PKR 0, not a failure.
	h := newFinanceHarness(t)
	driver := h.aUser(t)

	earnings, err := h.store.EarningsBetween(context.Background(), driver,
		time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("a driver with no history errored: %v", err)
	}
	if earnings.Net.Minor != 0 || earnings.Trips != 0 {
		t.Errorf("earnings = %+v, want zero", earnings)
	}
}

func TestAnInvertedWindowIsRefusedRatherThanAnsweredWithZero(t *testing.T) {
	// Zero would look like "you earned nothing", which is a different and
	// alarming statement.
	h := newFinanceHarness(t)
	now := time.Now()
	if _, err := h.store.EarningsBetween(context.Background(), h.aUser(t), now, now.Add(-time.Hour)); err == nil {
		t.Fatal("an inverted window was answered")
	}
}

func TestTripEarningsListWhatMakesUpTheTotal(t *testing.T) {
	// A driver checking earnings is usually checking one trip they think was
	// underpaid, and a total alone cannot answer that.
	h := newFinanceHarness(t)
	ctx := context.Background()
	customer := h.aUser(t)
	driver := h.aUser(t)
	for _, gross := range []int64{30000, 45000} {
		h.anEarning(t, driver, h.aJob(t, customer), gross, gross/5)
	}

	trips, err := h.store.TripEarningsSince(ctx, driver, time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(trips) != 2 {
		t.Fatalf("trips = %d, want 2", len(trips))
	}
	// Newest first: a driver looks for the trip they just finished.
	if !trips[0].At.After(trips[1].At) && !trips[0].At.Equal(trips[1].At) {
		t.Error("trips are not newest first")
	}
	for _, trip := range trips {
		if trip.Amount.Minor <= 0 {
			t.Errorf("trip amount = %d, want a positive figure a driver recognises",
				trip.Amount.Minor)
		}
		if trip.JobID == "" {
			t.Error("a trip earning has no job to point at")
		}
	}
}

func TestTheTripListIsBounded(t *testing.T) {
	h := newFinanceHarness(t)
	trips, err := h.store.TripEarningsSince(context.Background(), h.aUser(t),
		time.Now().Add(-time.Hour), 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(trips) > finance.MaxTripEarnings {
		t.Errorf("returned %d trips, want at most %d", len(trips), finance.MaxTripEarnings)
	}
}
