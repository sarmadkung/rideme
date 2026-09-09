//go:build integration

package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/merchant"
)

// aShopOwnedBy is aShop with a caller-chosen owner, which is what the merchant
// surface resolves a queue through.
func (h *merchantHarness) aShopOwnedBy(t *testing.T, ownerID, name string) shop {
	t.Helper()
	ctx := context.Background()
	var s shop
	if err := h.pool.QueryRow(ctx,
		`INSERT INTO merchants (owner_user_id, name, status) VALUES ($1, $2, 'ACTIVE')
		 RETURNING id::text`, ownerID, name).Scan(&s.merchantID); err != nil {
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

// aPlacedOrder walks the documented path — cart, line, place — rather than
// inserting a row, so the order under test is one the platform could produce.
func (h *merchantHarness) aPlacedOrder(t *testing.T, s shop) merchant.Order {
	t.Helper()
	ctx := context.Background()
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
	return placed
}

func TestMerchantByOwnerFindsTheShopThatAccountOperates(t *testing.T) {
	h := newMerchantHarness(t)
	owner := h.aUser(t)
	s := h.aShopOwnedBy(t, owner, "Al-Fatah")

	found, err := h.store.MerchantByOwner(context.Background(), owner)
	if err != nil {
		t.Fatalf("an owner could not find their own shop: %v", err)
	}
	if found.ID != s.merchantID || found.Name != "Al-Fatah" {
		t.Fatalf("found = %+v, want %s", found, s.merchantID)
	}
	if found.Status != merchant.StatusActive {
		t.Errorf("status = %q; the surface refuses actions unless it is active", found.Status)
	}
}

func TestAnAccountWithNoShopIsNotFound(t *testing.T) {
	// The MERCHANT role can be granted before onboarding finishes. That is a
	// different answer from "not permitted", and the surface says so.
	h := newMerchantHarness(t)

	_, err := h.store.MerchantByOwner(context.Background(), h.aUser(t))
	if !errors.Is(err, merchant.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestTwoShopsUnderOneAccountAreRefusedRatherThanPicked(t *testing.T) {
	// owner_user_id is indexed, not unique, so this is representable. Picking
	// one would show an owner the wrong shop's queue, and an order accepted
	// from the wrong queue is one nobody can fulfil.
	h := newMerchantHarness(t)
	owner := h.aUser(t)
	h.aShopOwnedBy(t, owner, "Al-Fatah Gulberg")
	h.aShopOwnedBy(t, owner, "Al-Fatah DHA")

	_, err := h.store.MerchantByOwner(context.Background(), owner)
	if !errors.Is(err, merchant.ErrManyMerchants) {
		t.Fatalf("err = %v, want ErrManyMerchants", err)
	}
}

func TestTheNewQueueHoldsPlacedOrdersNewestFirst(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	owner := h.aUser(t)
	s := h.aShopOwnedBy(t, owner, "Queue Test")

	first := h.aPlacedOrder(t, s)
	second := h.aPlacedOrder(t, s)

	statuses, _ := merchant.QueueNew.Statuses()
	found, err := h.store.OrdersFor(ctx, s.merchantID, statuses, nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("queue = %d orders, want 2", len(found))
	}
	// Newest first: a merchant works the top of the list, and the order that
	// arrived most recently is the one whose deadline they have most of.
	if found[0].ID != second.ID || found[1].ID != first.ID {
		t.Errorf("order = %s, %s; want %s, %s", found[0].ID, found[1].ID, second.ID, first.ID)
	}
	if found[0].AcceptDeadline == nil {
		t.Error("the queue lost the acceptance deadline")
	}
	// The queue query deliberately does not join lines (Store.OrdersFor).
	if len(found[0].Items) != 0 {
		t.Errorf("the queue carried %d lines", len(found[0].Items))
	}
}

func TestTheQueueNeverShowsAnotherMerchantsOrders(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	mine := h.aShopOwnedBy(t, h.aUser(t), "Mine")
	theirs := h.aShopOwnedBy(t, h.aUser(t), "Theirs")

	own := h.aPlacedOrder(t, mine)
	h.aPlacedOrder(t, theirs)

	statuses, _ := merchant.QueueNew.Statuses()
	found, err := h.store.OrdersFor(ctx, mine.merchantID, statuses, nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != own.ID {
		t.Fatalf("queue = %+v, want only %s", found, own.ID)
	}
}

func TestAcceptingMovesAnOrderOutOfTheNewQueue(t *testing.T) {
	// The two queues together are the merchant's whole working day, so what
	// leaves one has to arrive in the other.
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShopOwnedBy(t, h.aUser(t), "Moving Test")
	order := h.aPlacedOrder(t, s)

	if _, err := h.store.Transition(ctx, order.ID,
		merchant.StatusPlaced, merchant.StatusConfirmed, "MERCHANT", s.merchantID, nil); err != nil {
		t.Fatal(err)
	}

	newStatuses, _ := merchant.QueueNew.Statuses()
	inNew, err := h.store.OrdersFor(ctx, s.merchantID, newStatuses, nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(inNew) != 0 {
		t.Errorf("an accepted order is still in New: %+v", inNew)
	}

	preparing, _ := merchant.QueuePreparing.Statuses()
	inPreparing, err := h.store.OrdersFor(ctx, s.merchantID, preparing, nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(inPreparing) != 1 || inPreparing[0].ID != order.ID {
		t.Fatalf("Preparing = %+v, want %s", inPreparing, order.ID)
	}
	if inPreparing[0].AcceptedAt == nil {
		t.Error("accepted_at was not recorded (document 072)")
	}
}

func TestTheQueueCursorPagesWithoutRepeating(t *testing.T) {
	h := newMerchantHarness(t)
	ctx := context.Background()
	s := h.aShopOwnedBy(t, h.aUser(t), "Paging Test")
	h.aPlacedOrder(t, s)
	h.aPlacedOrder(t, s)

	statuses, _ := merchant.QueueNew.Statuses()
	firstPage, err := h.store.OrdersFor(ctx, s.merchantID, statuses, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage) != 1 {
		t.Fatalf("first page = %d", len(firstPage))
	}

	cursor := firstPage[0].CreatedAt
	secondPage, err := h.store.OrdersFor(ctx, s.merchantID, statuses, &cursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage) != 1 {
		t.Fatalf("second page = %d", len(secondPage))
	}
	if secondPage[0].ID == firstPage[0].ID {
		t.Error("the cursor returned the same order twice")
	}
	if !secondPage[0].CreatedAt.Before(cursor) {
		t.Error("the second page is not strictly older than the cursor")
	}
}
