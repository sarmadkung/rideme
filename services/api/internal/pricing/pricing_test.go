package pricing_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

var now = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

func engine() *pricing.Engine {
	return pricing.NewEngine(func() time.Time { return now })
}

// rideTariff is test configuration, not platform configuration. Every number
// here is invented for the test and none of it ships: BD-01, BD-02 and BD-05
// are unresolved, and the engine holds no rates of its own.
func rideTariff() pricing.Tariff {
	return pricing.Tariff{
		ID: "tariff-1", JobType: "RIDE", VehicleType: "CAR", Version: 1,
		Currency:        money.PKR,
		BaseMinor:       5000, // Rs 50
		PerKMMinor:      3000, // Rs 30/km
		PerMinuteMinor:  200,  // Rs 2/min
		ServiceFeeMinor: 1000, // Rs 10
		DemandMinBPS:    10000,
		DemandMaxBPS:    10000,
	}
}

func total(t *testing.T, q pricing.Quote) int64 {
	t.Helper()
	var sum int64
	for _, line := range q.Lines {
		sum += line.Amount.Minor
	}
	if sum != q.Total.Minor {
		t.Fatalf("lines sum to %d but total is %d — the breakdown does not explain the fare", sum, q.Total.Minor)
	}
	return q.Total.Minor
}

func TestARideQuoteFollowsDocument05sFormula(t *testing.T) {
	// base + distance + time (+ demand, inert) + service fee.
	quote, err := engine().Quote(pricing.Request{
		JobType: "RIDE", VehicleType: "CAR",
		DistanceMeters: 10000, DurationSeconds: 1200,
	}, rideTariff())
	if err != nil {
		t.Fatal(err)
	}

	// 5000 base + 30000 distance (10km) + 4000 time (20min) + 1000 fee.
	if got := total(t, quote); got != 40000 {
		t.Fatalf("total = %d, want 40000", got)
	}
	for _, want := range []pricing.Component{
		pricing.ComponentBase, pricing.ComponentDistance,
		pricing.ComponentTime, pricing.ComponentServiceFee,
	} {
		if _, ok := quote.Component(want); !ok {
			t.Errorf("%s missing from the breakdown", want)
		}
	}
}

func TestTheBreakdownAlwaysExplainsTheTotal(t *testing.T) {
	// Document 34 requires a complete breakdown. A total that does not equal
	// its lines is a fare nobody can dispute or reconstruct.
	cases := []pricing.Request{
		{JobType: "RIDE", DistanceMeters: 1, DurationSeconds: 1},
		{JobType: "RIDE", DistanceMeters: 999, DurationSeconds: 59},
		{JobType: "RIDE", DistanceMeters: 250000, DurationSeconds: 18000},
		{JobType: "RIDE"},
	}
	for _, req := range cases {
		quote, err := engine().Quote(req, rideTariff())
		if err != nil {
			t.Fatal(err)
		}
		total(t, quote)
	}
}

func TestNoFloatingPointDriftAcrossManyQuotes(t *testing.T) {
	// The reason BD-07 exists. Awkward distances that would round badly in a
	// float must sum exactly in integers.
	tariff := rideTariff()
	var sum int64
	for meters := int64(1); meters <= 3333; meters++ {
		quote, err := engine().Quote(pricing.Request{
			JobType: "RIDE", DistanceMeters: meters,
		}, tariff)
		if err != nil {
			t.Fatal(err)
		}
		total(t, quote)
		sum += quote.Total.Minor
	}
	if sum <= 0 {
		t.Fatal("accumulated nothing")
	}
}

func TestDistanceRoundsOnceAndHalfAwayFromZero(t *testing.T) {
	tariff := rideTariff()
	tariff.BaseMinor, tariff.ServiceFeeMinor, tariff.PerMinuteMinor = 0, 0, 0
	tariff.PerKMMinor = 1000 // Rs 10/km, so 1 metre is 1 minor unit

	cases := map[int64]int64{
		1:     1,    // one metre, one minor unit
		1500:  1500, // exact
		10000: 10000,
	}
	for meters, want := range cases {
		quote, err := engine().Quote(pricing.Request{JobType: "RIDE", DistanceMeters: meters}, tariff)
		if err != nil {
			t.Fatal(err)
		}
		if quote.Total.Minor != want {
			t.Errorf("%dm -> %d, want %d", meters, quote.Total.Minor, want)
		}
	}

	// A rate that does not divide evenly must still round exactly once.
	tariff.PerKMMinor = 333
	quote, err := engine().Quote(pricing.Request{JobType: "RIDE", DistanceMeters: 1500}, tariff)
	if err != nil {
		t.Fatal(err)
	}
	if quote.Total.Minor != 500 { // 333 * 1.5 = 499.5 -> 500 half away from zero
		t.Fatalf("333 per km over 1.5km = %d, want 500", quote.Total.Minor)
	}
}

func TestMinimumFareTopsUpAShortTrip(t *testing.T) {
	tariff := rideTariff()
	tariff.MinimumFareMinor = 15000 // Rs 150

	quote, err := engine().Quote(pricing.Request{
		JobType: "RIDE", DistanceMeters: 500, DurationSeconds: 120,
	}, tariff)
	if err != nil {
		t.Fatal(err)
	}
	if got := total(t, quote); got != 15000 {
		t.Fatalf("total = %d, want the 15000 minimum", got)
	}
	if _, ok := quote.Component(pricing.ComponentMinimumTopUp); !ok {
		t.Error("the top-up is not visible in the breakdown")
	}
}

func TestMinimumFareDoesNotReduceALongTrip(t *testing.T) {
	tariff := rideTariff()
	tariff.MinimumFareMinor = 15000

	quote, err := engine().Quote(pricing.Request{
		JobType: "RIDE", DistanceMeters: 20000, DurationSeconds: 2400,
	}, tariff)
	if err != nil {
		t.Fatal(err)
	}
	if quote.Total.Minor <= 15000 {
		t.Fatalf("a long trip was reduced to the minimum: %d", quote.Total.Minor)
	}
	if _, ok := quote.Component(pricing.ComponentMinimumTopUp); ok {
		t.Error("a top-up appeared on a fare above the minimum")
	}
}

func TestDemandIsInertByDefault(t *testing.T) {
	// BD-02 is unresolved. Document 05 includes demand in the formula; the
	// register says ship without surge and keep the term present.
	quote, err := engine().Quote(pricing.Request{
		JobType: "RIDE", DistanceMeters: 10000,
	}, rideTariff())
	if err != nil {
		t.Fatal(err)
	}
	if amount, ok := quote.Component(pricing.ComponentDemand); ok && !amount.IsZero() {
		t.Fatalf("demand adjusted the fare by %d with no surge configured", amount.Minor)
	}
}

func TestDemandIsBoundedByTheTariff(t *testing.T) {
	// Document 34: "Do not introduce uncontrolled surge." The bound is a
	// constraint, not a convention — an out-of-range multiplier is refused.
	tariff := rideTariff()
	tariff.DemandMaxBPS = 15000 // at most 1.5x

	within, err := engine().Quote(pricing.Request{
		JobType: "RIDE", DistanceMeters: 10000, DemandBPS: 12500,
	}, tariff)
	if err != nil {
		t.Fatal(err)
	}
	adjustment, ok := within.Component(pricing.ComponentDemand)
	if !ok || adjustment.IsZero() {
		t.Fatal("a configured demand multiplier had no effect")
	}
	total(t, within)

	if _, err := engine().Quote(pricing.Request{
		JobType: "RIDE", DistanceMeters: 10000, DemandBPS: 30000,
	}, tariff); !errors.Is(err, pricing.ErrDemandUnbounded) {
		t.Fatalf("a 3x multiplier was accepted against a 1.5x cap: %v", err)
	}
	// Below 1.0 is equally refused: surge must not become a silent discount.
	if _, err := engine().Quote(pricing.Request{
		JobType: "RIDE", DistanceMeters: 10000, DemandBPS: 5000,
	}, tariff); !errors.Is(err, pricing.ErrDemandUnbounded) {
		t.Fatalf("a 0.5x multiplier was accepted: %v", err)
	}
}

func TestTaxAppliesLastToWhatTheCustomerPays(t *testing.T) {
	tariff := rideTariff()
	tariff.TaxBPS = 1600 // 16%

	quote, err := engine().Quote(pricing.Request{
		JobType: "RIDE", DistanceMeters: 10000, DurationSeconds: 1200,
	}, tariff)
	if err != nil {
		t.Fatal(err)
	}
	tax, ok := quote.Component(pricing.ComponentTax)
	if !ok {
		t.Fatal("no tax line")
	}
	// 16% of the 40000 subtotal.
	if tax.Minor != 6400 {
		t.Fatalf("tax = %d, want 6400", tax.Minor)
	}
	if got := total(t, quote); got != 46400 {
		t.Fatalf("total = %d, want 46400", got)
	}
}

func TestADiscountNeverMakesAFareNegative(t *testing.T) {
	// The platform would owe the customer money for taking a ride.
	quote, err := engine().Quote(pricing.Request{
		JobType: "RIDE", DistanceMeters: 1000, DiscountMinor: 999999,
	}, rideTariff())
	if err != nil {
		t.Fatal(err)
	}
	if quote.Total.IsNegative() {
		t.Fatalf("total = %d", quote.Total.Minor)
	}
	if !quote.Total.IsZero() {
		t.Fatalf("an over-large discount left %d rather than zero", quote.Total.Minor)
	}
	total(t, quote)
}

func TestQuotesCarryTheirTariffVersionAndExpiry(t *testing.T) {
	// Document 34's price lock depends on both: a historical price must never
	// be recomputed from current configuration.
	quote, err := engine().Quote(pricing.Request{JobType: "RIDE", DistanceMeters: 5000}, rideTariff())
	if err != nil {
		t.Fatal(err)
	}
	if quote.PricingVersion != 1 || quote.TariffID != "tariff-1" {
		t.Fatalf("provenance lost: version=%d tariff=%s", quote.PricingVersion, quote.TariffID)
	}
	if !quote.ExpiresAt.Equal(now.Add(pricing.QuoteTTL)) {
		t.Fatalf("expiry = %v", quote.ExpiresAt)
	}
}

func TestAnUnpricedServiceIsRefusedNotGuessed(t *testing.T) {
	// Parcel, cargo and grocery arrive with their slices. Pricing one now
	// would mean inventing its rule set.
	for _, jobType := range []string{"PARCEL", "CARGO", "GROCERY", "FREIGHT", "TAXI"} {
		if _, err := engine().Quote(pricing.Request{JobType: jobType, DistanceMeters: 1000},
			rideTariff()); !errors.Is(err, pricing.ErrUnknownService) {
			t.Errorf("%s was priced by the ride rule set: %v", jobType, err)
		}
	}
}

func TestAServiceIsAddedByRuleSetNotByANewEngine(t *testing.T) {
	// This is the property CAP-1 exists to guarantee. Phase 9 adds parcel by
	// registering components, and nothing outside the map changes.
	pricing.Register("PARCEL_TEST",
		pricing.BaseRule, pricing.DistanceRule, pricing.WeightRule, pricing.ServiceFeeRule)

	tariff := rideTariff()
	tariff.JobType, tariff.PerKGMinor = "PARCEL_TEST", 500 // Rs 5/kg

	quote, err := engine().Quote(pricing.Request{
		JobType: "PARCEL_TEST", DistanceMeters: 4000, WeightKG: 6,
	}, tariff)
	if err != nil {
		t.Fatal(err)
	}
	weight, ok := quote.Component(pricing.ComponentWeight)
	if !ok || weight.Minor != 3000 {
		t.Fatalf("weight component = %+v, want 3000", weight)
	}
	// The distance component is the same code the ride slice uses.
	if _, ok := quote.Component(pricing.ComponentDistance); !ok {
		t.Fatal("the shared distance rule was not applied")
	}
	total(t, quote)
}

func TestWaitingAndLoadingAreRecordedButUnpricedByDefault(t *testing.T) {
	// BD-13: build the event recording, leave the rates to the owner. A tariff
	// with zero rates prices the time at nothing without losing it.
	pricing.Register("CARGO_TEST",
		pricing.BaseRule, pricing.DistanceRule, pricing.LoadingRule, pricing.WaitingRule)

	tariff := rideTariff()
	tariff.JobType = "CARGO_TEST"

	quote, err := engine().Quote(pricing.Request{
		JobType: "CARGO_TEST", DistanceMeters: 20000,
		LoadingSeconds: 1800, WaitingSeconds: 900,
	}, tariff)
	if err != nil {
		t.Fatal(err)
	}
	if amount, ok := quote.Component(pricing.ComponentLoading); ok && !amount.IsZero() {
		t.Fatalf("loading was priced at %d with no rate configured", amount.Minor)
	}
	total(t, quote)

	// Once a rate is configured, the same rule prices it.
	tariff.LoadingPerMinuteMinor = 100
	priced, err := engine().Quote(pricing.Request{
		JobType: "CARGO_TEST", DistanceMeters: 20000, LoadingSeconds: 1800,
	}, tariff)
	if err != nil {
		t.Fatal(err)
	}
	loading, ok := priced.Component(pricing.ComponentLoading)
	if !ok || loading.Minor != 3000 { // 30 minutes at 100
		t.Fatalf("loading = %+v, want 3000", loading)
	}
}

func TestRouteConfidenceIsCarriedIntoTheQuote(t *testing.T) {
	// A fare built on a straight-line guess must not look like one built on a
	// measured route.
	quote, err := engine().Quote(pricing.Request{
		JobType: "RIDE", DistanceMeters: 5000, RouteConfidence: "estimated",
	}, rideTariff())
	if err != nil {
		t.Fatal(err)
	}
	if quote.RouteConfidence != "estimated" {
		t.Fatalf("confidence = %q", quote.RouteConfidence)
	}
}
