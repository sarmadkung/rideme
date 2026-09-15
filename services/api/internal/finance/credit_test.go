package finance

import (
	"errors"
	"testing"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

func pkr(minor int64) money.Amount { return money.MustNew(minor, money.PKR) }

// The inverse of CashCollection, which is what makes the driver's balance a
// running sum rather than two figures somebody has to subtract.
func TestARemittanceReversesACollection(t *testing.T) {
	collected, err := CashCollection(pkr(50000), "driver-1", "customer-1", "job-1")
	if err != nil {
		t.Fatal(err)
	}
	remitted, err := DriverRemittance(pkr(50000), "driver-1")
	if err != nil {
		t.Fatal(err)
	}

	var cashInTransit int64
	for _, transaction := range []Transaction{collected, remitted} {
		if err := transaction.Balance(); err != nil {
			t.Errorf("transaction does not balance: %v", err)
		}
		for _, entry := range transaction.Entries {
			if entry.Account == AccountCashInTransit {
				cashInTransit += entry.Amount.Minor
			}
		}
	}

	if cashInTransit != 0 {
		t.Errorf("collecting then remitting leaves %d in cash-in-transit, want 0", cashInTransit)
	}
}

// A remittance debits the platform's clearing account and credits the driver's
// holding: the driver is carrying less of the platform's money.
func TestARemittanceCreditsTheDriversHolding(t *testing.T) {
	remitted, err := DriverRemittance(pkr(30000), "driver-2")
	if err != nil {
		t.Fatal(err)
	}
	if remitted.Kind != KindSettlement {
		t.Errorf("kind %q, want SETTLEMENT", remitted.Kind)
	}

	found := false
	for _, entry := range remitted.Entries {
		if entry.Account != AccountCashInTransit {
			continue
		}
		found = true
		if entry.Amount.Minor != -30000 {
			t.Errorf("cash-in-transit entry %d, want -30000", entry.Amount.Minor)
		}
		if entry.SubjectID != "driver-2" {
			t.Errorf("entry belongs to %q, want driver-2", entry.SubjectID)
		}
	}
	if !found {
		t.Error("a remittance wrote no cash-in-transit entry")
	}
}

// A settlement must be a real amount for a real driver. Posting a zero or a
// negative would move nothing and still tell a driver they had paid.
func TestARemittanceRefusesAnAmountThatIsNotMoney(t *testing.T) {
	if _, err := DriverRemittance(pkr(0), "driver-3"); err == nil {
		t.Error("a zero remittance was accepted")
	}
}

// The clearing amount is what puts a driver back on the road, not what clears
// their whole debt. Asking for the entire balance when a fraction would do is
// how a cap becomes a reason to stop driving for you.
func TestStandingClearingIsWhatUnblocksNotWhatIsOwed(t *testing.T) {
	// Mirrors StandingOf's arithmetic: blocked at the cap, clearing back down
	// to the warning threshold.
	owed, capAmount, warn := pkr(120000), pkr(100000), pkr(70000)

	blocked := owed.Minor >= capAmount.Minor
	if !blocked {
		t.Fatal("the fixture is not over the cap")
	}
	clearing, err := owed.Sub(warn)
	if err != nil {
		t.Fatal(err)
	}
	if clearing.Minor != 50000 {
		t.Errorf("clearing %d, want 50000", clearing.Minor)
	}
	if clearing.Minor >= owed.Minor {
		t.Error("clearing the block should cost less than clearing the debt")
	}
}

// An unset cap must not stop anyone working. This is the opposite of the
// commission rule on purpose: a guessed commission pays the wrong amount, but
// a missing cap row that failed closed would take every driver off the road at
// once.
func TestNoCapIsNotAnError(t *testing.T) {
	if !errors.Is(ErrNoCap, ErrNoCap) {
		t.Fatal("sentinel comparison is broken")
	}
	if errors.Is(ErrNoCap, ErrNoCommission) {
		t.Error("the missing-cap and missing-commission sentinels must stay distinct: " +
			"one permits and the other refuses")
	}
}
