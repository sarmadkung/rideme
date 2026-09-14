//go:build integration

package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
)

// The link document 070 describes: "Order and delivery state remain separate
// and communicate through explicit events." An order produces a Job when it
// reaches READY_FOR_PICKUP. Until this existed the grocery lifecycle stopped
// at PREPARING — there was no address to deliver to, and then no code to make
// the job — so a grocery order could be picked and never left the shop.

// aShopReadyToTrade is a located shop with stock, owned by a known user, which
// is what the merchant surface resolves a queue and an action through.
func (h *merchantHarness) aShopReadyToTrade(t *testing.T, ownerID string, at point) shop {
	t.Helper()
	ctx := context.Background()
	s := h.aShopOwnedBy(t, ownerID, "Delivery Test")

	if _, err := h.pool.Exec(ctx,
		`UPDATE stores SET location = ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography,
		                   address = 'Main Boulevard, Gulberg III, Lahore'
		  WHERE id = $3`, at.lat, at.lon, s.storeID); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetInventory(ctx, s.storeID, s.productID, "", true, nil); err != nil {
		t.Fatal(err)
	}
	return s
}

// aPreparedOrder walks the documented path as far as a picker: placed with a
// destination, accepted, and being prepared.
func (h *merchantHarness) aPreparedOrder(t *testing.T, s shop, to merchant.Delivery) merchant.Order {
	t.Helper()
	ctx := context.Background()

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 2, merchant.PreferAsk); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Place(ctx, cart.ID, to, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Transition(ctx, cart.ID, merchant.StatusPlaced,
		merchant.StatusConfirmed, "MERCHANT", s.merchantID, nil); err != nil {
		t.Fatal(err)
	}
	prepared, err := h.store.Transition(ctx, cart.ID, merchant.StatusConfirmed,
		merchant.StatusPreparing, "MERCHANT", s.merchantID, nil)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func TestAReadyOrderBecomesADeliveryJob(t *testing.T) {
	h := newMerchantHarness(t)
	wire := newWireHarness(t)
	ctx := context.Background()

	shopAt := somewhereNew()
	owner := h.aUser(t)
	s := h.aShopReadyToTrade(t, owner, shopAt)
	to := merchant.Delivery{
		Address: "House 12, Street 4, Gulberg III, Lahore",
		Lat:     shopAt.lat + 0.004,
		Lon:     shopAt.lon,
		Notes:   "second gate",
	}
	order := h.aPreparedOrder(t, s, to)

	service := merchant.NewService(h.store).WithJobs(wire.jobs)
	ready, jobID, err := service.MarkReady(ctx, owner, order.ID)
	if err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if ready.Status != merchant.StatusReadyForPickup {
		t.Fatalf("order is %s", ready.Status)
	}
	if jobID == "" {
		t.Fatal("no delivery job was created")
	}

	delivery, err := wire.jobs.ByID(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.Type != jobs.TypeGrocery {
		t.Errorf("job type = %s, want GROCERY", delivery.Type)
	}
	if delivery.Status != jobs.StatusRequested {
		t.Errorf("job status = %s, want REQUESTED so a dispatch round finds it", delivery.Status)
	}
	if delivery.MerchantID != s.merchantID {
		t.Errorf("merchant = %q", delivery.MerchantID)
	}
	if delivery.RequesterUserID != order.CustomerUserID {
		t.Errorf("requester = %q, want the customer waiting at the other end", delivery.RequesterUserID)
	}

	pickup, ok := delivery.Pickup()
	if !ok {
		t.Fatal("the delivery has no pickup")
	}
	if diff := pickup.Location.Latitude - shopAt.lat; diff > 0.00001 || diff < -0.00001 {
		t.Errorf("pickup = %+v, want the shop", pickup.Location)
	}
	dropoff, ok := delivery.Dropoff()
	if !ok {
		t.Fatal("the delivery has no dropoff")
	}
	if dropoff.Address != to.Address {
		t.Errorf("dropoff address = %q, want what the customer gave at checkout", dropoff.Address)
	}

	// And the order points at it, so a dashboard can follow the driver.
	linked, err := h.store.OrderByID(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if linked.JobID != jobID {
		t.Errorf("order.job_id = %q, want %q", linked.JobID, jobID)
	}
}

func TestAReadyOrderIsOfferedToADriver(t *testing.T) {
	// The grocery lifecycle end to end: a shop finishes picking and a driver
	// is asked to collect it. Every step before this one was verified in
	// isolation; none of them joined up.
	h := newMerchantHarness(t)
	wire := newWireHarness(t)
	ctx := context.Background()

	shopAt := somewhereNew()
	pickup := jobs.Coordinate{Latitude: shopAt.lat, Longitude: shopAt.lon}
	// A rider with a GROCERY-capable vehicle, waiting by the shop.
	driverID := wire.aDispatchableDriverFor(t, pickup, "GROCERY")

	owner := h.aUser(t)
	s := h.aShopReadyToTrade(t, owner, shopAt)
	order := h.aPreparedOrder(t, s, merchant.Delivery{
		Address: "House 12, Gulberg III, Lahore",
		Lat:     shopAt.lat + 0.004,
		Lon:     shopAt.lon,
	})

	service := merchant.NewService(h.store).WithJobs(wire.jobs)
	_, jobID, err := service.MarkReady(ctx, owner, order.ID)
	if err != nil {
		t.Fatalf("MarkReady: %v", err)
	}

	if _, err := wire.runner.Round(ctx, 200); err != nil {
		t.Fatal(err)
	}

	assignment, err := wire.jobs.LiveAssignment(ctx, jobID)
	if err != nil {
		t.Fatalf("the delivery reached no driver: %v", err)
	}
	if assignment.DriverID != driverID {
		t.Errorf("offered to %s, want %s", assignment.DriverID, driverID)
	}
}

func TestOneOrderProducesOneDelivery(t *testing.T) {
	// Two drivers sent to collect one order is two drivers, one of whom drove
	// for nothing. The guard is a compare-and-set on the order's job column,
	// so it holds even if two dashboards tap Mark Ready together.
	h := newMerchantHarness(t)
	wire := newWireHarness(t)
	ctx := context.Background()

	shopAt := somewhereNew()
	owner := h.aUser(t)
	s := h.aShopReadyToTrade(t, owner, shopAt)
	order := h.aPreparedOrder(t, s, merchant.Delivery{
		Address: "House 12, Gulberg III, Lahore",
		Lat:     shopAt.lat + 0.004,
		Lon:     shopAt.lon,
	})

	service := merchant.NewService(h.store).WithJobs(wire.jobs)
	_, first, err := service.MarkReady(ctx, owner, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := service.MarkReady(ctx, owner, order.ID)
	if err != nil {
		t.Fatalf("the second call failed rather than answering with the first delivery: %v", err)
	}
	if second != first {
		t.Fatalf("two deliveries for one order: %s and %s", first, second)
	}

	var deliveries int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM jobs WHERE merchant_id = $1 AND type = 'GROCERY'`,
		s.merchantID).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if deliveries != 1 {
		t.Errorf("the shop has %d grocery jobs, want 1", deliveries)
	}
}

func TestAttachingASecondJobIsRefused(t *testing.T) {
	// The guard itself, directly: a job column that could be overwritten would
	// leave a driver holding a delivery the order no longer points at.
	h := newMerchantHarness(t)
	wire := newWireHarness(t)
	ctx := context.Background()

	shopAt := somewhereNew()
	owner := h.aUser(t)
	s := h.aShopReadyToTrade(t, owner, shopAt)
	order := h.aPreparedOrder(t, s, merchant.Delivery{
		Address: "House 12, Gulberg III, Lahore",
		Lat:     shopAt.lat + 0.004,
		Lon:     shopAt.lon,
	})

	service := merchant.NewService(h.store).WithJobs(wire.jobs)
	_, jobID, err := service.MarkReady(ctx, owner, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.AttachJob(ctx, order.ID, jobID); !errors.Is(err, merchant.ErrJobAlreadyAttached) {
		t.Fatalf("err = %v, want ErrJobAlreadyAttached", err)
	}
}

func TestTheOrderFollowsTheDeliveryToTheDoor(t *testing.T) {
	// The last link. Before this the order stopped at READY_FOR_PICKUP for
	// good: the delivery job moved through its own states as the driver
	// worked, and nothing told the shop or the customer that their shopping
	// had been collected, was on its way, or had arrived.
	h := newMerchantHarness(t)
	wire := newWireHarness(t)
	ctx := context.Background()

	shopAt := somewhereNew()
	owner := h.aUser(t)
	s := h.aShopReadyToTrade(t, owner, shopAt)
	order := h.aPreparedOrder(t, s, merchant.Delivery{
		Address: "House 12, Gulberg III, Lahore",
		Lat:     shopAt.lat + 0.004,
		Lon:     shopAt.lon,
	})

	service := merchant.NewService(h.store).WithJobs(wire.jobs)
	_, jobID, err := service.MarkReady(ctx, owner, order.ID)
	if err != nil {
		t.Fatalf("MarkReady: %v", err)
	}

	// The driver has the goods.
	if err := service.FollowDelivery(ctx, jobID, jobs.StatusInProgress); err != nil {
		t.Fatal(err)
	}
	moving, err := h.store.OrderByID(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moving.Status != merchant.StatusDelivering {
		t.Fatalf("order is %s, want DELIVERING", moving.Status)
	}

	// And PICKED_UP is in the history, because a customer asking when their
	// shopping left the shop is asking about that row.
	var pickedUp bool
	if err := h.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM order_status_history
		                 WHERE order_id = $1 AND to_status = 'PICKED_UP')`,
		order.ID).Scan(&pickedUp); err != nil {
		t.Fatal(err)
	}
	if !pickedUp {
		t.Error("the order jumped to DELIVERING with no record of being collected")
	}

	// It arrives.
	if err := service.FollowDelivery(ctx, jobID, jobs.StatusCompleted); err != nil {
		t.Fatal(err)
	}
	delivered, err := h.store.OrderByID(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Status != merchant.StatusDelivered {
		t.Fatalf("order is %s, want DELIVERED", delivered.Status)
	}
}

func TestARepeatedDeliveryEventIsHarmlessAgainstTheDatabase(t *testing.T) {
	h := newMerchantHarness(t)
	wire := newWireHarness(t)
	ctx := context.Background()

	shopAt := somewhereNew()
	owner := h.aUser(t)
	s := h.aShopReadyToTrade(t, owner, shopAt)
	order := h.aPreparedOrder(t, s, merchant.Delivery{
		Address: "House 12, Gulberg III, Lahore",
		Lat:     shopAt.lat + 0.004,
		Lon:     shopAt.lon,
	})
	service := merchant.NewService(h.store).WithJobs(wire.jobs)
	_, jobID, err := service.MarkReady(ctx, owner, order.ID)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := service.FollowDelivery(ctx, jobID, jobs.StatusCompleted); err != nil {
			t.Fatalf("repeat %d: %v", i, err)
		}
	}

	var rows int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM order_status_history WHERE order_id = $1 AND to_status = 'DELIVERED'`,
		order.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("the order was delivered %d times", rows)
	}
}
