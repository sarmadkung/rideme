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

func TestAMerchantWithNoConfigGetsThePlatformAcceptanceWindow(t *testing.T) {
	// BD-12, resolved: ten minutes, then the order cancels itself. A merchant
	// that has never configured anything still gets a working deadline, from
	// the platform default rather than from a constant in Go.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 2, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	placed, err := h.store.Place(ctx, cart.ID, now)
	if err != nil {
		t.Fatalf("an unconfigured merchant could not take an order: %v", err)
	}
	if placed.AcceptDeadline == nil {
		t.Fatal("no acceptance deadline was set")
	}
	if delta := placed.AcceptDeadline.Sub(now); delta < 9*time.Minute || delta > 11*time.Minute {
		t.Fatalf("deadline is %v from now, want the platform default of 10 minutes", delta)
	}
}

func TestAMerchantsOwnTimeoutOverridesThePlatformDefault(t *testing.T) {
	// The platform default is a fallback, not a ceiling. A merchant that knows
	// it answers in two minutes should be able to say so.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	timeout := 5 * time.Minute
	if err := h.store.SetConfig(ctx, merchant.Config{MerchantID: s.merchantID, AcceptTimeout: &timeout}); err != nil {
		t.Fatal(err)
	}
	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 2, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	placed, err := h.store.Place(ctx, cart.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if delta := placed.AcceptDeadline.Sub(now); delta < 4*time.Minute || delta > 6*time.Minute {
		t.Fatalf("deadline is %v from now, want the merchant's own 5 minutes", delta)
	}
}

func TestAnOverdueOrderCancelsItself(t *testing.T) {
	// BD-12's second half. Without a sweeper the deadline is a timestamp
	// nobody acts on, and an order sent to a closed store waits forever.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)
	customerID := h.aUser(t)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, customerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}
	placed := time.Now().UTC()
	if _, err := h.store.Place(ctx, cart.ID, placed); err != nil {
		t.Fatal(err)
	}

	// Nothing expires while the merchant still has time to answer.
	if _, err := h.store.ExpireOverdue(ctx, placed.Add(9*time.Minute), 100); err != nil {
		t.Fatal(err)
	}
	still, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.Status != merchant.StatusPlaced {
		t.Fatalf("an order expired at 9 minutes, inside its 10-minute window: %s", still.Status)
	}

	// Past the deadline it cancels, attributed to the system rather than to
	// the customer or the merchant.
	expired, err := h.store.ExpireOverdue(ctx, placed.Add(11*time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, o := range expired {
		if o.ID == cart.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("the overdue order was not expired")
	}

	after, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != merchant.StatusCancelled {
		t.Fatalf("status after expiry = %s, want CANCELLED", after.Status)
	}

	var by, reason string
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(cancelled_by, ''), COALESCE(cancelled_reason, '')
		   FROM orders WHERE id = $1`, cart.ID).Scan(&by, &reason); err != nil {
		t.Fatal(err)
	}
	if by != "SYSTEM" || reason != "MERCHANT_ACCEPT_TIMEOUT" {
		t.Fatalf("expiry recorded as by=%q reason=%q", by, reason)
	}
}

func TestAcceptingAndExpiringTheSameOrderCannotBothHappen(t *testing.T) {
	// The race that matters: a merchant tapping accept as the sweeper runs.
	// Compare-and-set on PLACED means exactly one of them takes effect.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAllow); err != nil {
		t.Fatal(err)
	}
	placed := time.Now().UTC()
	if _, err := h.store.Place(ctx, cart.ID, placed); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var accepted, swept bool
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := h.store.Transition(ctx, cart.ID, merchant.StatusPlaced, merchant.StatusConfirmed,
			"MERCHANT", "", nil)
		accepted = err == nil
	}()
	go func() {
		defer wg.Done()
		expired, err := h.store.ExpireOverdue(ctx, placed.Add(time.Hour), 100)
		if err != nil {
			return
		}
		for _, o := range expired {
			if o.ID == cart.ID {
				swept = true
			}
		}
	}()
	wg.Wait()

	if accepted == swept {
		t.Fatalf("accept and expiry both %v — exactly one must win", accepted)
	}

	final, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted && final.Status != merchant.StatusConfirmed {
		t.Fatalf("the merchant accepted but the order is %s", final.Status)
	}
	if swept && final.Status != merchant.StatusCancelled {
		t.Fatalf("the sweeper won but the order is %s", final.Status)
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

// aSubstitutedOrder places one item and substitutes it at a new price.
func (h *merchantHarness) aSubstitutedOrder(t *testing.T, s shop, substituteMinor int64,
	resolution string) (orderID string, originalTotal int64) {
	t.Helper()
	ctx := context.Background()

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

	substitute := money.MustNew(substituteMinor, money.PKR)
	difference, err := merchant.PriceDifference(item.UnitPrice, substitute, item.Quantity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.RecordIssue(ctx, merchant.Issue{
		OrderID: cart.ID, OrderItemID: item.ID, Reason: "OUT_OF_STOCK",
		Action: merchant.ActionSubstitute, SubstituteName: "Milk 1L (premium)",
		SubstitutePrice: &substitute, PriceDifference: &difference,
		Resolution: resolution,
	}, "SUBSTITUTED"); err != nil {
		t.Fatal(err)
	}
	return cart.ID, before.ItemsTotal.Minor
}

func TestADearerSubstituteRaisesWhatTheCustomerPays(t *testing.T) {
	// BD-11, resolved: the customer pays what the substitute actually costs.
	h := newMerchantHarness(t)
	s := h.aShop(t)

	orderID, original := h.aSubstitutedOrder(t, s, 40000, merchant.ResolutionCustomerAccepted)
	after, err := h.store.OrderByID(context.Background(), orderID)
	if err != nil {
		t.Fatal(err)
	}
	if original != 25000 {
		t.Fatalf("the fixture ordered %d minor units, expected 25000", original)
	}
	if after.ItemsTotal.Minor != 40000 {
		t.Fatalf("total after substitution = %d, want the substitute's 40000", after.ItemsTotal.Minor)
	}
}

func TestACheaperSubstituteLowersWhatTheCustomerPays(t *testing.T) {
	// The other direction matters just as much: the platform does not keep the
	// saving when a shopper buys something cheaper.
	h := newMerchantHarness(t)
	s := h.aShop(t)

	orderID, _ := h.aSubstitutedOrder(t, s, 18000, merchant.ResolutionCustomerAccepted)
	after, err := h.store.OrderByID(context.Background(), orderID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ItemsTotal.Minor != 18000 {
		t.Fatalf("total after a cheaper substitution = %d, want 18000", after.ItemsTotal.Minor)
	}
}

func TestAnUnansweredSubstitutionChargesNothingYet(t *testing.T) {
	// A proposal the customer has not accepted must not reach their bill.
	h := newMerchantHarness(t)
	s := h.aShop(t)

	orderID, original := h.aSubstitutedOrder(t, s, 40000, merchant.ResolutionPending)
	after, err := h.store.OrderByID(context.Background(), orderID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ItemsTotal.Minor != original {
		t.Fatalf("a pending substitution changed the total from %d to %d",
			original, after.ItemsTotal.Minor)
	}
}

func TestChargingTheSubstitutePriceDoesNotRewriteWhatWasOrdered(t *testing.T) {
	// BD-11 and document 74 together. The customer pays the substitute's
	// price, and the original line still says what they asked for — so the
	// order total and the order history can disagree about the amount without
	// either being wrong.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t)

	orderID, _ := h.aSubstitutedOrder(t, s, 40000, merchant.ResolutionCustomerAccepted)

	var price int64
	var name string
	if err := h.pool.QueryRow(ctx,
		`SELECT unit_price_minor, name_snapshot FROM order_items WHERE order_id = $1`,
		orderID).Scan(&price, &name); err != nil {
		t.Fatal(err)
	}
	if price != 25000 {
		t.Fatalf("the ordered line was repriced to %d; it must still read 25000", price)
	}
	if name != "Milk 1L" {
		t.Fatalf("the ordered line was renamed to %q", name)
	}

	// What was actually supplied, and the difference charged for it, live in
	// the issue row.
	var substitute, difference int64
	var substituteName string
	if err := h.pool.QueryRow(ctx,
		`SELECT substitute_unit_price_minor, price_difference_minor, substitute_name
		   FROM order_item_issues WHERE order_id = $1`, orderID).
		Scan(&substitute, &difference, &substituteName); err != nil {
		t.Fatal(err)
	}
	if substitute != 40000 || difference != 15000 || substituteName != "Milk 1L (premium)" {
		t.Fatalf("the substitution recorded price=%d difference=%d name=%q",
			substitute, difference, substituteName)
	}

	// And the total charged reflects the substitute, not the original.
	order, err := h.store.OrderByID(ctx, orderID)
	if err != nil {
		t.Fatal(err)
	}
	if order.ItemsTotal.Minor != 40000 {
		t.Fatalf("total = %d, want the substitute's 40000", order.ItemsTotal.Minor)
	}
}
