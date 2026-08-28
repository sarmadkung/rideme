package pricing

import "testing"

// capped is BD-02 as decided: demand-triggered, ceiling 1.5x, and no surge
// computed below one available driver.
var capped = DemandPolicy{MaxBPS: 15000, MinSupply: 1}

func TestNoSurgeWhenSupplyMeetsDemand(t *testing.T) {
	cases := []Supply{
		{WaitingRequests: 0, AvailableDrivers: 10},
		{WaitingRequests: 5, AvailableDrivers: 10},
		{WaitingRequests: 10, AvailableDrivers: 10},
	}
	for _, s := range cases {
		if got := capped.MultiplierBPS(s); got != NeutralBPS {
			t.Fatalf("%d waiting against %d drivers gave %d bps, want neutral",
				s.WaitingRequests, s.AvailableDrivers, got)
		}
	}
}

func TestSurgeScalesWithTheRatio(t *testing.T) {
	// 12 waiting against 10 drivers is 1.2x.
	if got := capped.MultiplierBPS(Supply{WaitingRequests: 12, AvailableDrivers: 10}); got != 12000 {
		t.Fatalf("got %d bps, want 12000", got)
	}
	// 3 against 2 is exactly the cap.
	if got := capped.MultiplierBPS(Supply{WaitingRequests: 3, AvailableDrivers: 2}); got != 15000 {
		t.Fatalf("got %d bps, want 15000", got)
	}
}

func TestSurgeIsCappedHoweverExtremeTheShortage(t *testing.T) {
	for _, waiting := range []int{20, 200, 20000} {
		got := capped.MultiplierBPS(Supply{WaitingRequests: waiting, AvailableDrivers: 1})
		if got != 15000 {
			t.Fatalf("%d waiting against 1 driver gave %d bps, want the 15000 cap", waiting, got)
		}
	}
}

func TestNoSurgeWithoutSupplyToMeasureAgainst(t *testing.T) {
	// Zero drivers is not infinite demand — it is no signal. Dividing by it
	// would be a crash; treating it as maximum surge would multiply fares
	// every time the location pipeline hiccuped.
	if got := capped.MultiplierBPS(Supply{WaitingRequests: 50, AvailableDrivers: 0}); got != NeutralBPS {
		t.Fatalf("zero drivers gave %d bps, want neutral", got)
	}
}

func TestMinSupplyGateSuppressesThinMarkets(t *testing.T) {
	thin := DemandPolicy{MaxBPS: 15000, MinSupply: 5}
	// Four drivers is below the gate, so no surge however many are waiting.
	if got := thin.MultiplierBPS(Supply{WaitingRequests: 40, AvailableDrivers: 4}); got != NeutralBPS {
		t.Fatalf("got %d bps below the supply gate, want neutral", got)
	}
	// Five is at the gate, so the ratio applies and hits the cap.
	if got := thin.MultiplierBPS(Supply{WaitingRequests: 40, AvailableDrivers: 5}); got != 15000 {
		t.Fatalf("got %d bps at the supply gate, want the cap", got)
	}
}

func TestMultiplierNeverExceedsTheAbsoluteCeiling(t *testing.T) {
	// A settings row asking for 5x cannot produce one. LoadDemandPolicy clamps
	// on read; this asserts the clamp is the platform constant, so a bad edit
	// undercharges rather than overcharges.
	if AbsoluteMaxBPS != 15000 {
		t.Fatalf("the platform ceiling is %d bps, not the decided 15000", AbsoluteMaxBPS)
	}
}
