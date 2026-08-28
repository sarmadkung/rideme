package merchant_test

import (
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

func TestTheDocumentedOrderFlowIsWalkable(t *testing.T) {
	// Document 070's flow, end to end.
	flow := []merchant.OrderStatus{
		merchant.StatusCart, merchant.StatusPlaced, merchant.StatusPaymentPending,
		merchant.StatusConfirmed, merchant.StatusPreparing, merchant.StatusReadyForPickup,
		merchant.StatusPickedUp, merchant.StatusDelivering, merchant.StatusDelivered,
	}
	for i := 0; i < len(flow)-1; i++ {
		if err := merchant.Machine.Validate(flow[i], flow[i+1]); err != nil {
			t.Fatalf("%s -> %s should be allowed: %v", flow[i], flow[i+1], err)
		}
	}
}

func TestAnOrderCannotSkipMerchantFulfilment(t *testing.T) {
	refused := []struct{ from, to merchant.OrderStatus }{
		{merchant.StatusCart, merchant.StatusDelivered},
		{merchant.StatusPlaced, merchant.StatusReadyForPickup}, // ready without preparing
		{merchant.StatusConfirmed, merchant.StatusPickedUp},    // picked up without being ready
		{merchant.StatusDelivered, merchant.StatusCancelled},   // un-delivering
		{merchant.StatusCancelled, merchant.StatusConfirmed},   // resurrecting
	}
	for _, tc := range refused {
		if err := merchant.Machine.Validate(tc.from, tc.to); err == nil {
			t.Errorf("%s -> %s should be refused", tc.from, tc.to)
		}
	}
}

func TestMerchantRejectionIsOnlyPossibleBeforePreparation(t *testing.T) {
	// Document 070: "Merchant rejection may occur before preparation." Once a
	// picker has started, stock and staff time are consumed and the resolution
	// is operational rather than a rejection.
	for _, status := range []merchant.OrderStatus{
		merchant.StatusPlaced, merchant.StatusPaymentPending, merchant.StatusConfirmed,
	} {
		if !merchant.MerchantCancellable(status) {
			t.Errorf("a merchant should be able to reject at %s", status)
		}
	}
	for _, status := range []merchant.OrderStatus{
		merchant.StatusPreparing, merchant.StatusReadyForPickup,
		merchant.StatusPickedUp, merchant.StatusDelivered,
	} {
		if merchant.MerchantCancellable(status) {
			t.Errorf("a merchant should not be able to reject at %s", status)
		}
	}
}

func TestCustomerCancellationDependsOnState(t *testing.T) {
	// Document 070. Once picking starts, cancelling wastes goods someone has
	// already handled.
	if !merchant.CustomerCancellable(merchant.StatusPlaced) {
		t.Error("a customer should be able to cancel a placed order")
	}
	if merchant.CustomerCancellable(merchant.StatusPreparing) {
		t.Error("a customer cancelled an order already being picked")
	}
	if merchant.CustomerCancellable(merchant.StatusDelivering) {
		t.Error("a customer cancelled an order already out for delivery")
	}
}

func TestTheCustomersSubstitutionPreferenceIsAuthoritative(t *testing.T) {
	// Document 74. A merchant proposing a substitution for an item marked
	// DO_NOT_ALLOW does not get to make it — the customer receives a partial
	// order rather than something they explicitly refused.
	if got := merchant.ResolveIssue(merchant.PreferDoNotAllow, merchant.ActionSubstitute); got != merchant.ActionRemove {
		t.Errorf("DO_NOT_ALLOW + SUBSTITUTE -> %s, want REMOVE", got)
	}
	// ALLOW lets the merchant proceed.
	if got := merchant.ResolveIssue(merchant.PreferAllow, merchant.ActionSubstitute); got != merchant.ActionSubstitute {
		t.Errorf("ALLOW + SUBSTITUTE -> %s, want SUBSTITUTE", got)
	}
	// ASK_ME turns a substitution into a question, not a decision.
	if got := merchant.ResolveIssue(merchant.PreferAsk, merchant.ActionSubstitute); got != merchant.ActionAsk {
		t.Errorf("ASK_ME + SUBSTITUTE -> %s, want REQUEST_CUSTOMER_DECISION", got)
	}
	// Removal needs no permission: it is what happens when nothing can be
	// supplied.
	for _, preference := range []merchant.SubstitutionPreference{
		merchant.PreferAllow, merchant.PreferDoNotAllow, merchant.PreferAsk,
	} {
		if got := merchant.ResolveIssue(preference, merchant.ActionRemove); got != merchant.ActionRemove {
			t.Errorf("%s + REMOVE -> %s, want REMOVE", preference, got)
		}
	}
}

func TestAnUnknownPreferenceAsksRatherThanAssumes(t *testing.T) {
	if got := merchant.ResolveIssue("", merchant.ActionSubstitute); got != merchant.ActionAsk {
		t.Fatalf("an unset preference produced %s; it should ask", got)
	}
}

func TestOrderTotalsAreExact(t *testing.T) {
	items := []merchant.Item{
		{UnitPrice: money.MustNew(12550, money.PKR), Quantity: 3},
		{UnitPrice: money.MustNew(999, money.PKR), Quantity: 7},
		{UnitPrice: money.MustNew(1, money.PKR), Quantity: 99},
	}
	total, err := merchant.Total(money.PKR, items)
	if err != nil {
		t.Fatal(err)
	}
	// 37650 + 6993 + 99, in integers.
	if total.Minor != 44742 {
		t.Fatalf("total = %d, want 44742", total.Minor)
	}
}

func TestAnEmptyOrderTotalsToZeroNotAnError(t *testing.T) {
	total, err := merchant.Total(money.PKR, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !total.IsZero() || total.Currency != money.PKR {
		t.Fatalf("total = %+v", total)
	}
}

func TestStoreHoursDecideWhetherAStoreIsOpen(t *testing.T) {
	// Thursday 09:00–17:00.
	hours := []merchant.Hours{{Weekday: int(time.Thursday), OpensAt: 9 * 3600, ClosesAt: 17 * 3600}}

	thursday := func(hour, minute int) time.Time {
		// 2026-08-27 is a Thursday.
		return time.Date(2026, 8, 27, hour, minute, 0, 0, time.UTC)
	}
	if !merchant.StoreOpenAt(hours, thursday(12, 0)) {
		t.Error("closed at midday inside the window")
	}
	if !merchant.StoreOpenAt(hours, thursday(9, 0)) {
		t.Error("closed at exactly the opening time")
	}
	// Closing time is exclusive: an order placed at 17:00:00 is after hours.
	if merchant.StoreOpenAt(hours, thursday(17, 0)) {
		t.Error("open at exactly the closing time")
	}
	if merchant.StoreOpenAt(hours, thursday(8, 59)) {
		t.Error("open a minute before opening")
	}
	// A different weekday is not covered by Thursday's window.
	if merchant.StoreOpenAt(hours, time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)) {
		t.Error("Thursday's hours opened the store on Friday")
	}
}

func TestNoStoreHoursMeansClosed(t *testing.T) {
	// Failing closed is right: a store with no configured hours has not said
	// it is open, and taking orders it cannot fulfil is worse than taking none.
	if merchant.StoreOpenAt(nil, time.Now()) {
		t.Fatal("a store with no configured hours was treated as open")
	}
}
