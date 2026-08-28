//go:build integration

package tests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

type merchantHarness struct {
	store *merchant.Store
	pool  *pgxpool.Pool
}

func newMerchantHarness(t *testing.T) *merchantHarness {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return &merchantHarness{store: merchant.NewStore(pool), pool: pool}
}

func (h *merchantHarness) aUser(t *testing.T) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (phone) VALUES ('+9241' || lpad((floor(random()*100000000))::text, 8, '0'))
		 RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type shop struct {
	merchantID, storeID, productID string
}

func (h *merchantHarness) aShop(t *testing.T) shop {
	t.Helper()
	ctx := context.Background()
	var s shop
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO merchants (owner_user_id, name, status) VALUES ($1, 'Test Kiryana', 'ACTIVE')
		 RETURNING id::text`, h.aUser(t)).Scan(&s.merchantID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO stores (merchant_id, name) VALUES ($1, 'Main Branch') RETURNING id::text`,
		s.merchantID).Scan(&s.storeID); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO products (merchant_id, name, price_minor, status)
		 VALUES ($1, 'Milk 1L', 25000, 'ACTIVE') RETURNING id::text`,
		s.merchantID).Scan(&s.productID); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPlacingAnOrderRequiresAConfiguredAcceptanceTimeout(t *testing.T) {
	// BD-12 is unresolved. The register asks for "an explicit unset state that
	// fails loudly rather than defaulting silently" — a guessed timeout would
	// either auto-cancel orders merchants were about to accept, or leave
	// customers waiting indefinitely.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)
	customerID := h.aUser(t)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, customerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 2, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}

	if _, err := h.store.Place(ctx, cart.ID, time.Now()); !errors.Is(err, merchant.ErrAcceptTimeoutUnset) {
		t.Fatalf("an order was placed with no configured timeout: %v", err)
	}

	// Configuring one unblocks it, and the deadline comes from that value.
	timeout := 5 * time.Minute
	if err := h.store.SetConfig(ctx, merchant.Config{MerchantID: s.merchantID, AcceptTimeout: &timeout}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	placed, err := h.store.Place(ctx, cart.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if placed.AcceptDeadline == nil {
		t.Fatal("no acceptance deadline was set")
	}
	if delta := placed.AcceptDeadline.Sub(now); delta < 4*time.Minute || delta > 6*time.Minute {
		t.Fatalf("deadline is %v from now, want the configured 5 minutes", delta)
	}
}

func TestAnEmptyCartCannotBePlaced(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)
	timeout := time.Minute
	if err := h.store.SetConfig(ctx, merchant.Config{MerchantID: s.merchantID, AcceptTimeout: &timeout}); err != nil {
		t.Fatal(err)
	}
	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Place(ctx, cart.ID, time.Now()); err == nil {
		t.Fatal("an empty cart was placed")
	}
}

func TestOpeningACartTwiceReturnsTheSameCart(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)
	customerID := h.aUser(t)

	first, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, customerID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, customerID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != again.ID {
		t.Fatal("a second cart was created for the same customer and store")
	}
}

func TestOrderLinesSnapshotThePriceAtCheckout(t *testing.T) {
	// Document 68: "Later catalog changes must never alter historical orders."
	// Referencing the live price would rewrite every past receipt the next
	// time a merchant edited one.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	item, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 2, merchant.PreferAllow)
	if err != nil {
		t.Fatal(err)
	}
	if item.UnitPrice.Minor != 25000 {
		t.Fatalf("unit price = %d", item.UnitPrice.Minor)
	}

	// The merchant doubles the price.
	if _, err := h.pool.Exec(ctx,
		`UPDATE products SET price_minor = 50000 WHERE id = $1`, s.productID); err != nil {
		t.Fatal(err)
	}

	reloaded, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Items[0].UnitPrice.Minor != 25000 {
		t.Fatalf("the stored line moved with the catalogue: %d", reloaded.Items[0].UnitPrice.Minor)
	}
	if reloaded.ItemsTotal.Minor != 50000 {
		t.Fatalf("total = %d, want 2 × 25000", reloaded.ItemsTotal.Minor)
	}
}

func TestTheOrderTotalAlwaysMatchesItsLines(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	var second string
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO products (merchant_id, name, price_minor, status)
		 VALUES ($1, 'Bread', 8000, 'ACTIVE') RETURNING id::text`, s.merchantID).Scan(&second); err != nil {
		t.Fatal(err)
	}

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 3, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, second, "", 2, merchant.PreferAsk); err != nil {
		t.Fatal(err)
	}

	order, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	computed, err := merchant.Total(money.PKR, order.Items)
	if err != nil {
		t.Fatal(err)
	}
	if computed.Minor != order.ItemsTotal.Minor {
		t.Fatalf("stored total %d does not match the lines' %d", order.ItemsTotal.Minor, computed.Minor)
	}
	if order.ItemsTotal.Minor != 91000 { // 75000 + 16000
		t.Fatalf("total = %d", order.ItemsTotal.Minor)
	}
}

func TestAnInactiveProductCannotBeAdded(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	if _, err := h.pool.Exec(ctx,
		`UPDATE products SET status = 'OUT_OF_STOCK' WHERE id = $1`, s.productID); err != nil {
		t.Fatal(err)
	}
	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAllow); !errors.Is(err, merchant.ErrOutOfStock) {
		t.Fatalf("an out-of-stock product was added: %v", err)
	}
}

func TestTheAcceptanceTimeoutSweepCancelsUnansweredOrders(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)
	timeout := 30 * time.Second
	if err := h.store.SetConfig(ctx, merchant.Config{MerchantID: s.merchantID, AcceptTimeout: &timeout}); err != nil {
		t.Fatal(err)
	}

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Place(ctx, cart.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	if _, err := h.store.SweepAcceptTimeouts(ctx, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	order, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != merchant.StatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", order.Status)
	}
	// The cancellation is on the record, not silent.
	var historyCount int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM order_status_history WHERE order_id = $1 AND to_status = 'CANCELLED'`,
		cart.ID).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 {
		t.Fatalf("%d cancellation history rows", historyCount)
	}
}

func TestAnAcceptedOrderIsNotSweptAway(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)
	timeout := 30 * time.Second
	if err := h.store.SetConfig(ctx, merchant.Config{MerchantID: s.merchantID, AcceptTimeout: &timeout}); err != nil {
		t.Fatal(err)
	}

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Place(ctx, cart.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Transition(ctx, cart.ID, merchant.StatusPlaced, merchant.StatusConfirmed,
		"MERCHANT", "", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := h.store.SweepAcceptTimeouts(ctx, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	order, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != merchant.StatusConfirmed {
		t.Fatalf("an accepted order was swept to %s", order.Status)
	}
	// Acceptance stamped its timestamp (document 072).
	if order.AcceptedAt == nil {
		t.Fatal("accepted_at was not recorded")
	}
}

func TestMerchantTimestampsAreRecordedByTheTransitions(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)
	timeout := time.Minute
	if err := h.store.SetConfig(ctx, merchant.Config{MerchantID: s.merchantID, AcceptTimeout: &timeout}); err != nil {
		t.Fatal(err)
	}

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Place(ctx, cart.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	for _, step := range []struct{ from, to merchant.OrderStatus }{
		{merchant.StatusPlaced, merchant.StatusConfirmed},
		{merchant.StatusConfirmed, merchant.StatusPreparing},
		{merchant.StatusPreparing, merchant.StatusReadyForPickup},
	} {
		if _, err := h.store.Transition(ctx, cart.ID, step.from, step.to, "MERCHANT", "", nil); err != nil {
			t.Fatalf("%s -> %s: %v", step.from, step.to, err)
		}
	}

	order, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Document 072 lists all three.
	if order.AcceptedAt == nil || order.PreparationStart == nil || order.ReadyAt == nil {
		t.Fatalf("timestamps missing: %+v", order)
	}
}

func TestConcurrentOrderTransitionsProduceOneWinner(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)
	timeout := time.Minute
	if err := h.store.SetConfig(ctx, merchant.Config{MerchantID: s.merchantID, AcceptTimeout: &timeout}); err != nil {
		t.Fatal(err)
	}

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Place(ctx, cart.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	// A merchant accepting while the timeout sweep cancels: the real race.
	const racers = 6
	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := h.store.Transition(ctx, cart.ID, merchant.StatusPlaced, merchant.StatusConfirmed,
				"MERCHANT", "", nil)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var won int
	for err := range results {
		if err == nil {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("%d of %d concurrent acceptances succeeded, want 1", won, racers)
	}
}

func TestInventoryCannotBeOversoldUnderConcurrency(t *testing.T) {
	// Document 69's definition of done: "inventory reservation cannot oversell
	// under concurrent orders." The last unit of anything is exactly where two
	// customers meet.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	stock := 5
	if err := h.store.SetInventory(ctx, s.storeID, s.productID, "", true, &stock); err != nil {
		t.Fatal(err)
	}

	const racers = 20
	var wg sync.WaitGroup
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- h.store.Reserve(ctx, s.storeID, s.productID, "", 1)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var reserved int
	for err := range results {
		if err == nil {
			reserved++
		} else if !errors.Is(err, merchant.ErrOutOfStock) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if reserved != stock {
		t.Fatalf("%d of 20 reservations succeeded against %d units", reserved, stock)
	}

	var reservedQuantity, quantity int
	if err := h.pool.QueryRow(ctx,
		`SELECT reserved_quantity, quantity FROM inventory
		  WHERE store_id = $1 AND product_id = $2 AND variant_id IS NULL`,
		s.storeID, s.productID).Scan(&reservedQuantity, &quantity); err != nil {
		t.Fatal(err)
	}
	if reservedQuantity > quantity {
		t.Fatalf("oversold: %d reserved against %d units", reservedQuantity, quantity)
	}
}

func TestReleasingAReservationReturnsStock(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	stock := 2
	if err := h.store.SetInventory(ctx, s.storeID, s.productID, "", true, &stock); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := h.store.Reserve(ctx, s.storeID, s.productID, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.store.Reserve(ctx, s.storeID, s.productID, "", 1); !errors.Is(err, merchant.ErrOutOfStock) {
		t.Fatalf("sold a third unit of two: %v", err)
	}

	// A cancelled order releases its hold (document 69).
	if err := h.store.ReleaseReservation(ctx, s.storeID, s.productID, "", 1); err != nil {
		t.Fatal(err)
	}
	if err := h.store.Reserve(ctx, s.storeID, s.productID, "", 1); err != nil {
		t.Fatalf("the released unit could not be re-reserved: %v", err)
	}
}

func TestAnUnavailableItemCannotBeReserved(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	if err := h.store.SetInventory(ctx, s.storeID, s.productID, "", false, nil); err != nil {
		t.Fatal(err)
	}
	if err := h.store.Reserve(ctx, s.storeID, s.productID, "", 1); !errors.Is(err, merchant.ErrOutOfStock) {
		t.Fatalf("an unavailable item was reserved: %v", err)
	}
}

func TestASubstitutionNeverMutatesTheOriginalLine(t *testing.T) {
	// Document 74: "Never mutate the original order-line history." What the
	// customer ordered must stay readable after what they received changed.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	item, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 2, merchant.PreferAllow)
	if err != nil {
		t.Fatal(err)
	}

	substitute := money.MustNew(27000, money.PKR)
	difference := money.MustNew(2000, money.PKR)
	issue, err := h.store.RecordIssue(ctx, merchant.Issue{
		OrderID: cart.ID, OrderItemID: item.ID, Reason: "OUT_OF_STOCK",
		Action: merchant.ActionSubstitute, SubstituteName: "Milk 1L (other brand)",
		SubstitutePrice: &substitute, PriceDifference: &difference,
		Resolution: "CUSTOMER_ACCEPTED",
	}, "SUBSTITUTED")
	if err != nil {
		t.Fatal(err)
	}
	if issue.ID == "" {
		t.Fatal("the issue was not recorded")
	}

	order, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The original line keeps its name and its price; only its status moved.
	if order.Items[0].NameSnapshot != "Milk 1L" {
		t.Fatalf("the original line was renamed to %q", order.Items[0].NameSnapshot)
	}
	if order.Items[0].UnitPrice.Minor != 25000 {
		t.Fatalf("the original line's price changed to %d", order.Items[0].UnitPrice.Minor)
	}
	if order.Items[0].Status != "SUBSTITUTED" {
		t.Fatalf("item status = %s", order.Items[0].Status)
	}

	issues, err := h.store.IssuesOf(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].SubstituteName != "Milk 1L (other brand)" {
		t.Fatalf("the substitution was not recorded: %+v", issues)
	}
}

func TestARemovedItemLeavesTheOrderTotal(t *testing.T) {
	// Document 74: removed items "may create a partial refund". The total must
	// stop including what nobody received.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	item, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 2, merchant.PreferDoNotAllow)
	if err != nil {
		t.Fatal(err)
	}

	before, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.ItemsTotal.Minor != 50000 {
		t.Fatalf("total before = %d", before.ItemsTotal.Minor)
	}

	if _, err := h.store.RecordIssue(ctx, merchant.Issue{
		OrderID: cart.ID, OrderItemID: item.ID, Reason: "OUT_OF_STOCK",
		Action: merchant.ActionRemove, Resolution: "AUTO_APPLIED",
	}, "REMOVED"); err != nil {
		t.Fatal(err)
	}

	after, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ItemsTotal.Minor != 0 {
		t.Fatalf("total after removal = %d, want 0", after.ItemsTotal.Minor)
	}
	// The line is still there, marked, not deleted.
	if len(after.Items) != 1 || after.Items[0].Status != "REMOVED" {
		t.Fatalf("the removed line was deleted rather than marked: %+v", after.Items)
	}
}

func TestNoPriceDifferenceIsChargedAnywhere(t *testing.T) {
	// BD-11 is unresolved: who absorbs a substitution's price difference is a
	// product decision. The difference is recorded; nothing charges it.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	item, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAllow)
	if err != nil {
		t.Fatal(err)
	}
	before, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}

	substitute := money.MustNew(40000, money.PKR)
	difference := money.MustNew(15000, money.PKR)
	if _, err := h.store.RecordIssue(ctx, merchant.Issue{
		OrderID: cart.ID, OrderItemID: item.ID, Reason: "OUT_OF_STOCK",
		Action: merchant.ActionSubstitute, SubstituteName: "Milk 1L (premium)",
		SubstitutePrice: &substitute, PriceDifference: &difference,
	}, ""); err != nil {
		t.Fatal(err)
	}

	after, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ItemsTotal.Minor != before.ItemsTotal.Minor {
		t.Fatalf("a substitution changed the order total from %d to %d with BD-11 unresolved",
			before.ItemsTotal.Minor, after.ItemsTotal.Minor)
	}
}
