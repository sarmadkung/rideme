package finance_test

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

func pkr(minor int64) money.Amount { return money.MustNew(minor, money.PKR) }

func sum(t *testing.T, tx finance.Transaction) int64 {
	t.Helper()
	var total int64
	for _, entry := range tx.Entries {
		total += entry.Amount.Minor
	}
	return total
}

func TestEveryBuiltTransactionBalances(t *testing.T) {
	// Document 53: "Each transaction must balance." This is the invariant the
	// whole ledger rests on — an unbalanced entry means money appeared or
	// vanished, and no report afterwards can tell you where.
	payment, err := finance.CustomerPayment(pkr(45000), "cust-1", "job-1", "intent-1")
	if err != nil {
		t.Fatal(err)
	}
	earning, err := finance.DriverEarning(pkr(45000), pkr(9000), "driver-1", "job-1")
	if err != nil {
		t.Fatal(err)
	}
	refund, err := finance.Refund(pkr(45000), "cust-1", "job-1", "intent-1")
	if err != nil {
		t.Fatal(err)
	}
	cash, err := finance.CashCollection(pkr(45000), "driver-1", "cust-1", "job-1")
	if err != nil {
		t.Fatal(err)
	}
	payout, err := finance.Payout(pkr(36000), finance.SubjectDriver, "driver-1")
	if err != nil {
		t.Fatal(err)
	}

	for name, tx := range map[string]finance.Transaction{
		"payment": payment, "earning": earning, "refund": refund,
		"cash": cash, "payout": payout,
	} {
		if err := tx.Balance(); err != nil {
			t.Errorf("%s does not balance: %v", name, err)
		}
		if got := sum(t, tx); got != 0 {
			t.Errorf("%s entries sum to %d", name, got)
		}
	}
}

func TestBalanceRejectsWhatDoesNotBalance(t *testing.T) {
	unbalanced := finance.Transaction{Entries: []finance.Entry{
		finance.Debit(finance.AccountCustomerReceivable, pkr(100), finance.SubjectCustomer, "c1"),
		finance.Debit(finance.AccountPlatformClearing, pkr(100), finance.SubjectPlatform, ""),
	}}
	if err := unbalanced.Balance(); !errors.Is(err, finance.ErrUnbalanced) {
		t.Fatalf("want ErrUnbalanced, got %v", err)
	}

	single := finance.Transaction{Entries: []finance.Entry{
		finance.Debit(finance.AccountCustomerReceivable, pkr(100), finance.SubjectCustomer, "c1"),
	}}
	if err := single.Balance(); !errors.Is(err, finance.ErrNoEntries) {
		t.Fatalf("want ErrNoEntries, got %v", err)
	}
}

func TestATransactionCannotMixCurrencies(t *testing.T) {
	// Two currencies summing to zero by coincidence would balance numerically
	// and mean nothing.
	credit, err := finance.Credit(finance.AccountPlatformClearing, pkr(100), finance.SubjectPlatform, "")
	if err != nil {
		t.Fatal(err)
	}
	mixed := finance.Transaction{Entries: []finance.Entry{
		{Account: finance.AccountCustomerReceivable, Amount: money.Amount{Minor: 100, Currency: "USD"}},
		credit,
	}}
	if err := mixed.Balance(); !errors.Is(err, finance.ErrMixedCurrency) {
		t.Fatalf("want ErrMixedCurrency, got %v", err)
	}
}

func TestDriverEarningSplitsGrossExactly(t *testing.T) {
	// The split must lose nothing: net plus commission is the gross, to the
	// paisa, or a driver is short by an amount nobody can account for.
	gross, commission := pkr(100000), pkr(12500)
	tx, err := finance.DriverEarning(gross, commission, "driver-1", "job-1")
	if err != nil {
		t.Fatal(err)
	}

	var payable, revenue, expense int64
	for _, entry := range tx.Entries {
		switch entry.Account {
		case finance.AccountDriverPayable:
			payable = -entry.Amount.Minor
		case finance.AccountPlatformRevenue:
			revenue = -entry.Amount.Minor
		case finance.AccountDriverExpense:
			expense = entry.Amount.Minor
		}
	}
	if expense != gross.Minor {
		t.Fatalf("expense = %d, want the gross %d", expense, gross.Minor)
	}
	if payable+revenue != gross.Minor {
		t.Fatalf("net %d + commission %d = %d, want %d", payable, revenue, payable+revenue, gross.Minor)
	}
}

func TestCommissionCannotExceedTheEarning(t *testing.T) {
	// A driver must never owe money for working.
	if _, err := finance.DriverEarning(pkr(1000), pkr(5000), "driver-1", "job-1"); err == nil {
		t.Fatal("a commission larger than the gross was accepted")
	}
}

func TestAZeroCommissionProducesTwoEntries(t *testing.T) {
	// A zero revenue line carries no information and would clutter every
	// report; the transaction still balances without it.
	tx, err := finance.DriverEarning(pkr(50000), pkr(0), "driver-1", "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.Entries) != 2 {
		t.Fatalf("%d entries with zero commission, want 2", len(tx.Entries))
	}
	if err := tx.Balance(); err != nil {
		t.Fatal(err)
	}
}

func TestCommissionIsComputedFromConfiguredIntegerRates(t *testing.T) {
	// BD-05 is a PRODUCT_DECISION; the rate is configuration and expressed in
	// basis points, never as a float.
	rate := finance.CommissionRate{RateBPS: 1250} // 12.5%
	amount, err := finance.Commission(pkr(100000), rate)
	if err != nil {
		t.Fatal(err)
	}
	if amount.Minor != 12500 {
		t.Fatalf("commission = %d, want 12500", amount.Minor)
	}

	// Rounding matches the fare engine's — half away from zero, once.
	odd, err := finance.Commission(pkr(101), finance.CommissionRate{RateBPS: 1250})
	if err != nil {
		t.Fatal(err)
	}
	if odd.Minor != 13 { // 12.625 -> 13
		t.Fatalf("commission on 101 = %d, want 13", odd.Minor)
	}
}

func TestCommissionWithAFlatComponentIsCapped(t *testing.T) {
	rate := finance.CommissionRate{RateBPS: 1000, Flat: pkr(5000)}
	amount, err := finance.Commission(pkr(100000), rate)
	if err != nil {
		t.Fatal(err)
	}
	if amount.Minor != 15000 { // 10000 + 5000
		t.Fatalf("commission = %d, want 15000", amount.Minor)
	}
	// On a tiny fare the flat fee would exceed the earning; it is capped.
	capped, err := finance.Commission(pkr(1000), rate)
	if err != nil {
		t.Fatal(err)
	}
	if capped.Minor != 1000 {
		t.Fatalf("commission = %d, want it capped at the 1000 gross", capped.Minor)
	}
}

func TestAReversalIsTheExactInverse(t *testing.T) {
	// Document 53: corrections use new transactions, never edits. The pair
	// must net to zero and the original must stay readable.
	original, err := finance.CustomerPayment(pkr(45000), "cust-1", "job-1", "intent-1")
	if err != nil {
		t.Fatal(err)
	}
	original.ID = "txn-1"

	reversal, err := finance.Reverse(original, "duplicate capture")
	if err != nil {
		t.Fatal(err)
	}
	if err := reversal.Balance(); err != nil {
		t.Fatal(err)
	}
	if reversal.ReversesID != "txn-1" {
		t.Fatalf("the reversal does not reference the original: %+v", reversal)
	}
	if len(reversal.Entries) != len(original.Entries) {
		t.Fatalf("%d entries, want %d", len(reversal.Entries), len(original.Entries))
	}

	// Every account nets to zero across the pair.
	net := map[finance.Account]int64{}
	for _, entry := range append(append([]finance.Entry{}, original.Entries...), reversal.Entries...) {
		net[entry.Account] += entry.Amount.Minor
	}
	for account, total := range net {
		if total != 0 {
			t.Errorf("%s nets to %d after reversal, want 0", account, total)
		}
	}
}

func TestAnUnsavedTransactionCannotBeReversed(t *testing.T) {
	// A reversal that references nothing is an unexplained adjustment.
	original, err := finance.CustomerPayment(pkr(100), "c", "j", "i")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finance.Reverse(original, "oops"); err == nil {
		t.Fatal("a transaction with no id was reversed")
	}
}

func TestBalanceHoldsAcrossRandomSplits(t *testing.T) {
	// Property-style: whatever the amounts, no built transaction ever creates
	// or destroys a paisa.
	rng := rand.New(rand.NewSource(20260828))
	for i := 0; i < 2000; i++ {
		gross := rng.Int63n(10_000_000) + 1
		rate := finance.CommissionRate{RateBPS: int(rng.Int63n(10001))}

		commission, err := finance.Commission(pkr(gross), rate)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := finance.DriverEarning(pkr(gross), commission, "driver-1", "job-1")
		if err != nil {
			t.Fatalf("gross=%d rate=%d: %v", gross, rate.RateBPS, err)
		}
		if got := sum(t, tx); got != 0 {
			t.Fatalf("gross=%d rate=%d: entries sum to %d", gross, rate.RateBPS, got)
		}
	}
}

func TestCashCollectionAllocatesNoLiability(t *testing.T) {
	// BD-09 — who bears the loss if a driver never remits cash — is a legal
	// allocation and unresolved. The movement is recorded; nothing decides
	// whose loss it is.
	tx, err := finance.CashCollection(pkr(45000), "driver-1", "cust-1", "job-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range tx.Entries {
		if entry.Account == finance.AccountDriverPayable || entry.Account == finance.AccountPlatformRevenue {
			t.Fatalf("cash collection touched %s; BD-09 has not been decided", entry.Account)
		}
	}
	// It sits in transit, held by the driver, owned by nobody yet.
	var found bool
	for _, entry := range tx.Entries {
		if entry.Account == finance.AccountCashInTransit && entry.SubjectID == "driver-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("collected cash was not recorded against the driver holding it")
	}
}
