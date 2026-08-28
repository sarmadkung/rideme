package money_test

import (
	"encoding/json"
	"errors"
	"math/rand"
	"testing"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

func TestNewRejectsUnknownCurrency(t *testing.T) {
	if _, err := money.New(100, "USD"); !errors.Is(err, money.ErrUnknownCurrency) {
		t.Fatalf("want ErrUnknownCurrency, got %v", err)
	}
}

func TestNewRejectsAmountsBeyondTheSafeRange(t *testing.T) {
	// The bound exists because JavaScript clients hold every number as a
	// float64. One past it is where the client and the server would disagree.
	if _, err := money.New(money.MaxSafeMinor, money.PKR); err != nil {
		t.Fatalf("the bound itself must be valid: %v", err)
	}
	for _, minor := range []int64{money.MaxSafeMinor + 1, -money.MaxSafeMinor - 1} {
		if _, err := money.New(minor, money.PKR); !errors.Is(err, money.ErrOutOfRange) {
			t.Fatalf("minor=%d: want ErrOutOfRange, got %v", minor, err)
		}
	}
}

func TestAddSubRejectCurrencyMismatch(t *testing.T) {
	pkr := money.MustNew(100, money.PKR)
	other := money.Amount{Minor: 100, Currency: "USD"}

	if _, err := pkr.Add(other); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Add: want ErrCurrencyMismatch, got %v", err)
	}
	if _, err := pkr.Sub(other); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Sub: want ErrCurrencyMismatch, got %v", err)
	}
	if _, err := pkr.Compare(other); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Compare: want ErrCurrencyMismatch, got %v", err)
	}
}

func TestAddSubAreExact(t *testing.T) {
	// The canonical float failure: 0.1 + 0.2 != 0.3. In minor units it is
	// 10 + 20 == 30, exactly, every time.
	a := money.MustNew(10, money.PKR)
	b := money.MustNew(20, money.PKR)

	sum, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Minor != 30 {
		t.Fatalf("want 30, got %d", sum.Minor)
	}

	back, err := sum.Sub(b)
	if err != nil {
		t.Fatal(err)
	}
	if back != a {
		t.Fatalf("sub did not invert add: %v vs %v", back, a)
	}
}

func TestApplyRateRoundsHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		name     string
		minor    int64
		num, den int64
		want     int64
	}{
		{"exact division", 1000, 1, 4, 250},
		{"half rounds up", 5, 1, 2, 3},              // 2.5 -> 3
		{"negative half rounds away", -5, 1, 2, -3}, // -2.5 -> -3
		{"below half rounds down", 4, 1, 3, 1},      // 1.333 -> 1
		{"above half rounds up", 5, 2, 3, 3},        // 3.333 -> 3
		{"just under half", 199, 1, 100, 2},         // 1.99 -> 2
		{"zero stays zero", 0, 7, 3, 0},
		{"percentage as integers", 12345, 125, 1000, 1543}, // 12.5% of 12345 = 1543.125
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := money.MustNew(tc.minor, money.PKR).ApplyRate(tc.num, tc.den)
			if err != nil {
				t.Fatal(err)
			}
			if got.Minor != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got.Minor)
			}
		})
	}
}

func TestApplyRateRejectsZeroDenominator(t *testing.T) {
	if _, err := money.MustNew(100, money.PKR).ApplyRate(1, 0); !errors.Is(err, money.ErrDivideByZero) {
		t.Fatalf("want ErrDivideByZero, got %v", err)
	}
}

func TestAllocateConservesEveryMinorUnit(t *testing.T) {
	// The invariant that makes a ledger reconcilable: the parts sum to the
	// whole, with no remainder quietly dropped.
	cases := []struct {
		minor   int64
		weights []int64
	}{
		{100, []int64{1, 1, 1}},  // 33.33 each
		{10, []int64{1, 1, 1}},   // 3.33 each
		{1, []int64{1, 1, 1}},    // one unit across three parties
		{-100, []int64{1, 1, 1}}, // a reversal splits the same way
		{9999, []int64{70, 30}},  // an uneven commission split
		{5, []int64{0, 1}},       // a zero weight receives nothing
	}
	for _, tc := range cases {
		parts, err := money.MustNew(tc.minor, money.PKR).Allocate(tc.weights)
		if err != nil {
			t.Fatal(err)
		}
		if len(parts) != len(tc.weights) {
			t.Fatalf("want %d parts, got %d", len(tc.weights), len(parts))
		}
		var total int64
		for _, part := range parts {
			total += part.Minor
		}
		if total != tc.minor {
			t.Fatalf("minor=%d weights=%v: parts sum to %d", tc.minor, tc.weights, total)
		}
	}
}

func TestAllocateConservationHoldsForRandomSplits(t *testing.T) {
	// Property-style: whatever the amount and however it is weighted, nothing
	// is created and nothing is lost.
	rng := rand.New(rand.NewSource(20260828))
	for i := 0; i < 2000; i++ {
		minor := rng.Int63n(2_000_001) - 1_000_000
		weights := make([]int64, 1+rng.Intn(6))
		var weightTotal int64
		for j := range weights {
			weights[j] = rng.Int63n(100)
			weightTotal += weights[j]
		}
		if weightTotal == 0 {
			weights[0] = 1
		}

		parts, err := money.MustNew(minor, money.PKR).Allocate(weights)
		if err != nil {
			t.Fatalf("minor=%d weights=%v: %v", minor, weights, err)
		}
		var total int64
		for _, part := range parts {
			total += part.Minor
		}
		if total != minor {
			t.Fatalf("minor=%d weights=%v: parts sum to %d", minor, weights, total)
		}
	}
}

func TestAllocateRejectsUnusableWeights(t *testing.T) {
	a := money.MustNew(100, money.PKR)
	if _, err := a.Allocate([]int64{1, -1}); !errors.Is(err, money.ErrNegativeWeight) {
		t.Fatalf("want ErrNegativeWeight, got %v", err)
	}
	if _, err := a.Allocate([]int64{0, 0}); !errors.Is(err, money.ErrEmptyAllocation) {
		t.Fatalf("want ErrEmptyAllocation, got %v", err)
	}
	if _, err := a.Allocate(nil); !errors.Is(err, money.ErrEmptyAllocation) {
		t.Fatalf("want ErrEmptyAllocation, got %v", err)
	}
}

func TestSumIsExactAndCurrencyChecked(t *testing.T) {
	total, err := money.Sum(money.PKR,
		money.MustNew(1, money.PKR),
		money.MustNew(2, money.PKR),
		money.MustNew(-3, money.PKR),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !total.IsZero() {
		t.Fatalf("a balanced ledger must sum to zero, got %d", total.Minor)
	}

	if _, err := money.Sum(money.PKR, money.Amount{Minor: 1, Currency: "USD"}); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch, got %v", err)
	}
}

func TestMulIntOverflowIsReportedNotWrapped(t *testing.T) {
	if _, err := money.MustNew(money.MaxSafeMinor, money.PKR).MulInt(1000); !errors.Is(err, money.ErrOutOfRange) {
		t.Fatalf("want ErrOutOfRange, got %v", err)
	}
}

func TestJSONRoundTripIsDeterministic(t *testing.T) {
	amount := money.MustNew(-12345, money.PKR)

	encoded, err := json.Marshal(amount)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"amount_minor":-12345,"currency":"PKR"}`
	if string(encoded) != want {
		t.Fatalf("want %s, got %s", want, encoded)
	}

	// Same value, same bytes, every time.
	again, err := json.Marshal(amount)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(encoded) {
		t.Fatalf("encoding is not deterministic: %s vs %s", encoded, again)
	}

	var decoded money.Amount
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != amount {
		t.Fatalf("round trip changed the value: %v vs %v", decoded, amount)
	}
}

func TestUnmarshalRejectsInvalidAmounts(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"fractional minor units", `{"amount_minor":10.5,"currency":"PKR"}`},
		{"float that looks whole", `{"amount_minor":1e2,"currency":"PKR"}`},
		{"unknown currency", `{"amount_minor":10,"currency":"USD"}`},
		{"missing amount", `{"currency":"PKR"}`},
		{"beyond the safe range", `{"amount_minor":9007199254740992,"currency":"PKR"}`},
		{"string amount", `{"amount_minor":"10","currency":"PKR"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var decoded money.Amount
			if err := json.Unmarshal([]byte(tc.body), &decoded); err == nil {
				t.Fatalf("accepted invalid amount: %+v", decoded)
			}
		})
	}
}

func TestStringIsForDisplayOnly(t *testing.T) {
	cases := map[int64]string{
		0:      "PKR 0.00",
		5:      "PKR 0.05",
		150:    "PKR 1.50",
		-12345: "-PKR 123.45",
	}
	for minor, want := range cases {
		if got := money.MustNew(minor, money.PKR).String(); got != want {
			t.Fatalf("minor=%d: want %q, got %q", minor, want, got)
		}
	}
}
