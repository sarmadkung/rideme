//go:build integration

package tests

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/merchant"
)

// point is somewhere on the map, and every test below works relative to one of
// its own.
type point struct{ lat, lon float64 }

// somewhereNew is an unused patch of the map.
//
// These tests search by radius against a database that keeps every shop any
// previous run created. Two tests sharing a location would see each other's
// shops, and a fixed location would collect them run after run until a bounded
// result set stopped returning the one under test. A fresh point per shop makes
// each test the only thing near it.
func somewhereNew() point {
	return point{
		lat: 20 + rand.Float64()*5,
		lon: 60 + rand.Float64()*8,
	}
}

// aLocatedShop is aShop with coordinates, opening hours and stock — the three
// things the customer surface reads and the merchant fixtures never needed.
func (h *merchantHarness) aLocatedShop(t *testing.T, at point, open bool) shop {
	t.Helper()
	ctx := context.Background()
	lat, lon := at.lat, at.lon
	s := h.aShop(t)

	if _, err := h.pool.Exec(ctx,
		`UPDATE stores SET location = ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography
		  WHERE id = $3`, lat, lon, s.storeID); err != nil {
		t.Fatal(err)
	}
	if open {
		// Every day, all day: the test is about whether hours are applied, not
		// about a particular shop's timetable.
		for weekday := 0; weekday < 7; weekday++ {
			if _, err := h.pool.Exec(ctx,
				`INSERT INTO store_hours (store_id, weekday, opens_at, closes_at)
				 VALUES ($1, $2, '00:00', '23:59')`, s.storeID, weekday); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := h.store.SetInventory(ctx, s.storeID, s.productID, "", true, nil); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNearbyShopsComeBackNearestFirstWithTheirDistance(t *testing.T) {
	h := newMerchantHarness(t)
	here := somewhereNew()
	near := h.aLocatedShop(t, here, true)
	// Roughly 1.1 km north: 0.01 degrees of latitude.
	far := h.aLocatedShop(t, point{here.lat + 0.01, here.lon}, true)

	found, err := h.store.OutletsNear(context.Background(),
		here.lat, here.lon, 5000, time.Now(), 25)
	if err != nil {
		t.Fatal(err)
	}

	positions := map[string]int{}
	for i, outlet := range found {
		positions[outlet.ID] = i
	}
	nearAt, nearFound := positions[near.storeID]
	farAt, farFound := positions[far.storeID]
	if !nearFound || !farFound {
		t.Fatalf("both shops should be within 5km: %+v", positions)
	}
	if nearAt > farAt {
		t.Error("the further shop came first; distance is the only order a customer wants")
	}
	for _, outlet := range found {
		if outlet.ID == far.storeID && (outlet.DistanceM < 900 || outlet.DistanceM > 1300) {
			t.Errorf("distance = %v m, want roughly 1100", outlet.DistanceM)
		}
	}
}

func TestAShopOutsideTheRadiusIsNotOffered(t *testing.T) {
	h := newMerchantHarness(t)
	here := somewhereNew()
	// A degree of latitude is about 111 km.
	distant := h.aLocatedShop(t, point{here.lat + 1, here.lon}, true)

	found, err := h.store.OutletsNear(context.Background(),
		here.lat, here.lon, 5000, time.Now(), 25)
	if err != nil {
		t.Fatal(err)
	}
	for _, outlet := range found {
		if outlet.ID == distant.storeID {
			t.Fatal("a shop 111 km away was offered as nearby")
		}
	}
}

func TestAShopWithNoHoursReadsAsClosedRatherThanAbsent(t *testing.T) {
	// A customer looking for their usual kiryana needs to see that it is shut.
	// Hiding it looks like it closed down.
	h := newMerchantHarness(t)
	here := somewhereNew()
	shut := h.aLocatedShop(t, here, false)

	found, err := h.store.OutletsNear(context.Background(),
		here.lat, here.lon, 5000, time.Now(), 25)
	if err != nil {
		t.Fatal(err)
	}
	for _, outlet := range found {
		if outlet.ID != shut.storeID {
			continue
		}
		if outlet.Open {
			t.Error("a shop with no opening hours reported itself open")
		}
		return
	}
	t.Fatal("the shut shop was not listed at all")
}

func TestASuspendedMerchantsShopIsNotOffered(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	here := somewhereNew()
	s := h.aLocatedShop(t, here, true)
	if _, err := h.pool.Exec(ctx,
		`UPDATE merchants SET status = 'SUSPENDED' WHERE id = $1`, s.merchantID); err != nil {
		t.Fatal(err)
	}

	found, err := h.store.OutletsNear(ctx, here.lat, here.lon, 5000, time.Now(), 25)
	if err != nil {
		t.Fatal(err)
	}
	for _, outlet := range found {
		if outlet.ID == s.storeID {
			t.Fatal("a suspended merchant was taking orders")
		}
	}
}

func TestTheCatalogueReportsStockAtThisBranch(t *testing.T) {
	// Document 069: the same product is on the shelf in Gulberg and not in
	// DHA. Availability is the inventory row here, not the product's status.
	h := newMerchantHarness(t)
	ctx := context.Background()
	stocked := h.aLocatedShop(t, somewhereNew(), true)

	products, err := h.store.ProductsAt(ctx, stocked.storeID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) == 0 {
		t.Fatal("the shop's own product was not listed")
	}
	var found bool
	for _, product := range products {
		if product.ID != stocked.productID {
			continue
		}
		found = true
		if !product.Available {
			t.Error("a stocked product reported itself unavailable")
		}
		if product.Price.Minor != 25000 {
			t.Errorf("price = %d, want the catalogue price", product.Price.Minor)
		}
	}
	if !found {
		t.Fatal("the product was missing from its own shop's catalogue")
	}

	// Now take it off the shelf.
	if err := h.store.SetInventory(ctx, stocked.storeID, stocked.productID, "", false, nil); err != nil {
		t.Fatal(err)
	}
	products, err = h.store.ProductsAt(ctx, stocked.storeID, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, product := range products {
		if product.ID == stocked.productID && product.Available {
			t.Error("an item taken off the shelf still reported itself available")
		}
	}
}

func TestAProductWithNoInventoryRowIsNotOffered(t *testing.T) {
	// Reserve refuses a product with no inventory row, so offering it as
	// available is a cart that fails at checkout for no visible reason.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShop(t) // aShop writes no inventory

	products, err := h.store.ProductsAt(ctx, s.storeID, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, product := range products {
		if product.ID == s.productID && product.Available {
			t.Fatal("a product with no stock record was offered as available")
		}
	}
}

func TestCheckoutRecordsWhereTheOrderIsGoing(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aLocatedShop(t, somewhereNew(), true)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 2, merchant.PreferAsk); err != nil {
		t.Fatal(err)
	}

	placed, err := h.store.Place(ctx, cart.ID, aDestination(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if placed.Delivery.Address != aDestination().Address {
		t.Errorf("address = %q", placed.Delivery.Address)
	}
	// PostGIS stores geography as float8; a coordinate that comes back rounded
	// to the wrong street is worse than one that fails loudly.
	if diff := placed.Delivery.Lat - aDestination().Lat; diff > 0.00001 || diff < -0.00001 {
		t.Errorf("latitude = %v, want %v", placed.Delivery.Lat, aDestination().Lat)
	}
	if diff := placed.Delivery.Lon - aDestination().Lon; diff > 0.00001 || diff < -0.00001 {
		t.Errorf("longitude = %v, want %v", placed.Delivery.Lon, aDestination().Lon)
	}
	if placed.Delivery.Notes != aDestination().Notes {
		t.Errorf("notes = %q", placed.Delivery.Notes)
	}

	// And it survives a reload, which is the path every other reader takes.
	reloaded, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Delivery.Address != aDestination().Address {
		t.Errorf("reloaded address = %q", reloaded.Delivery.Address)
	}
}

func TestAnOrderWithNowhereToGoIsNotPlaced(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aLocatedShop(t, somewhereNew(), true)

	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAsk); err != nil {
		t.Fatal(err)
	}

	for _, destination := range []merchant.Delivery{
		{},
		{Address: "House 12, Gulberg"},
		{Lat: 31.5169, Lon: 74.3484},
		{Address: "House 12", Lat: 0, Lon: 0},
		{Address: "House 12", Lat: 91, Lon: 74},
	} {
		if _, err := h.store.Place(ctx, cart.ID, destination, time.Now().UTC()); !errors.Is(err, merchant.ErrNoDestination) {
			t.Errorf("destination %+v: err = %v, want ErrNoDestination", destination, err)
		}
	}

	// The cart is untouched, so the customer can fix the address and retry.
	unplaced, err := h.store.OrderByID(ctx, cart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unplaced.Status != merchant.StatusCart {
		t.Errorf("status = %s, want it still a cart", unplaced.Status)
	}
}

func TestHalfADestinationCannotBeStored(t *testing.T) {
	// The application refuses it, and so does the table: a name with no
	// coordinates cannot be routed to and coordinates with no name cannot be
	// read out to a driver (migration 000012).
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aLocatedShop(t, somewhereNew(), true)
	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := h.pool.Exec(ctx,
		`UPDATE orders SET delivery_address = 'House 12' WHERE id = $1`, cart.ID); err == nil {
		t.Error("an address with no coordinates was stored")
	}
	if _, err := h.pool.Exec(ctx,
		`UPDATE orders
		    SET delivery_location = ST_SetSRID(ST_MakePoint(74.3484, 31.5169), 4326)::geography
		  WHERE id = $1`, cart.ID); err == nil {
		t.Error("coordinates with no address were stored")
	}
}

func TestACustomerOnlySeesTheirOwnOrders(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aLocatedShop(t, somewhereNew(), true)

	mine := h.aUser(t)
	cart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, mine)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, cart.ID, s.productID, "", 1, merchant.PreferAsk); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Place(ctx, cart.ID, aDestination(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	// Somebody else's order at the same shop.
	theirCart, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, h.aUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AddItem(ctx, theirCart.ID, s.productID, "", 1, merchant.PreferAsk); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Place(ctx, theirCart.ID, aDestination(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	found, err := h.store.OrdersOf(ctx, mine, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != cart.ID {
		t.Fatalf("orders = %+v, want only %s", found, cart.ID)
	}
}

func TestACartIsNotOrderHistory(t *testing.T) {
	// An order list that includes a cart shows a customer something they have
	// not done yet, next to things they have.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aLocatedShop(t, somewhereNew(), true)
	customer := h.aUser(t)

	if _, err := h.store.OpenCart(ctx, s.merchantID, s.storeID, customer); err != nil {
		t.Fatal(err)
	}
	found, err := h.store.OrdersOf(ctx, customer, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("a cart appeared in order history: %+v", found)
	}
}
