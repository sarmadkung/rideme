package merchant_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

const (
	ownerID   = "user-1"
	shopID    = "merchant-1"
	otherShop = "merchant-2"
)

// stubStore stands in for Postgres. What is under test is the surface's own
// decisions — whose order this is, which action is legal from which state —
// not SQL, which the integration test covers.
type stubStore struct {
	shop        merchant.Merchant
	shopErr     error
	orders      []merchant.Order
	order       merchant.Order
	orderErr    error
	transitions []transition
	rejections  []rejection
	askedFor    []merchant.OrderStatus
}

type transition struct {
	orderID  string
	from, to merchant.OrderStatus
	actorID  string
}

type rejection struct {
	orderID string
	from    merchant.OrderStatus
	reason  string
}

func (s *stubStore) MerchantByOwner(_ context.Context, _ string) (merchant.Merchant, error) {
	if s.shopErr != nil {
		return merchant.Merchant{}, s.shopErr
	}
	return s.shop, nil
}

func (s *stubStore) OrdersFor(_ context.Context, _ string, statuses []merchant.OrderStatus,
	_ *time.Time, _ int) ([]merchant.Order, error) {
	s.askedFor = statuses
	return s.orders, nil
}

func (s *stubStore) OrderByID(_ context.Context, _ string) (merchant.Order, error) {
	if s.orderErr != nil {
		return merchant.Order{}, s.orderErr
	}
	return s.order, nil
}

func (s *stubStore) Transition(_ context.Context, orderID string, from, to merchant.OrderStatus,
	_, actorID string, _ map[string]any) (merchant.Order, error) {
	s.transitions = append(s.transitions, transition{orderID, from, to, actorID})
	moved := s.order
	moved.Status = to
	return moved, nil
}

func (s *stubStore) Reject(_ context.Context, orderID string, from merchant.OrderStatus,
	_, reason string) (merchant.Order, error) {
	s.rejections = append(s.rejections, rejection{orderID, from, reason})
	rejected := s.order
	rejected.Status = merchant.StatusCancelled
	rejected.RejectionReason = reason
	return rejected, nil
}

func anActiveShop() merchant.Merchant {
	return merchant.Merchant{ID: shopID, OwnerUserID: ownerID, Name: "Al-Fatah", Status: "ACTIVE"}
}

func anOrder(status merchant.OrderStatus) merchant.Order {
	return merchant.Order{
		ID:             "order-1",
		MerchantID:     shopID,
		CustomerUserID: "customer-9",
		Status:         status,
		ItemsTotal:     money.MustNew(120000, money.PKR),
		CreatedAt:      time.Now(),
		Items: []merchant.Item{{
			ID:           "item-1",
			NameSnapshot: "Olper's milk 1L",
			UnitPrice:    money.MustNew(60000, money.PKR),
			Quantity:     2,
			Preference:   merchant.PreferAsk,
			Status:       "ORDERED",
		}},
	}
}

// serve wires the routes with a principal already attached: whether the token
// pipeline works is identity's concern, not this package's.
func serve(store merchant.OrderStore, roles ...identity.Role) *http.ServeMux {
	mux := http.NewServeMux()
	merchant.NewHandler(merchant.NewService(store)).Routes(mux,
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := identity.ContextWithPrincipal(r.Context(),
					identity.Principal{UserID: ownerID, Roles: roles})
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
	return mux
}

func call(t *testing.T, mux *http.ServeMux, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))
	return recorder
}

func TestTheQueueAsksForTheStatesThatQueueHolds(t *testing.T) {
	// The mapping is a decision, not a translation — eleven states into five
	// queues — so it is asserted rather than assumed.
	for _, testCase := range []struct {
		queue string
		want  []merchant.OrderStatus
	}{
		{"", []merchant.OrderStatus{merchant.StatusPlaced, merchant.StatusPaymentPending}},
		{"new", []merchant.OrderStatus{merchant.StatusPlaced, merchant.StatusPaymentPending}},
		{"preparing", []merchant.OrderStatus{merchant.StatusConfirmed, merchant.StatusPreparing}},
		{"ready", []merchant.OrderStatus{merchant.StatusReadyForPickup}},
		{"completed", []merchant.OrderStatus{
			merchant.StatusPickedUp, merchant.StatusDelivering, merchant.StatusDelivered}},
		{"cancelled", []merchant.OrderStatus{merchant.StatusCancelled, merchant.StatusFailed}},
	} {
		store := &stubStore{shop: anActiveShop()}
		target := "/api/v1/merchant/orders"
		if testCase.queue != "" {
			target += "?queue=" + testCase.queue
		}
		response := call(t, serve(store, identity.RoleMerchant), http.MethodGet, target, "")
		if response.Code != http.StatusOK {
			t.Fatalf("queue %q: status = %d, body %s", testCase.queue, response.Code, response.Body)
		}
		if len(store.askedFor) != len(testCase.want) {
			t.Fatalf("queue %q asked for %v, want %v", testCase.queue, store.askedFor, testCase.want)
		}
		for i, status := range testCase.want {
			if store.askedFor[i] != status {
				t.Errorf("queue %q asked for %v, want %v", testCase.queue, store.askedFor, testCase.want)
			}
		}
	}
}

func TestAnUnknownQueueIsRefusedRatherThanEmptied(t *testing.T) {
	// An empty list would read as "no orders", which is a different and much
	// more alarming statement than "you asked for a queue that does not exist".
	store := &stubStore{shop: anActiveShop()}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodGet,
		"/api/v1/merchant/orders?queue=urgent", "")

	if response.Code != http.StatusUnprocessableEntity && response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
}

func TestTheQueueCarriesTheDeadlineAndNoLines(t *testing.T) {
	deadline := time.Now().Add(9 * time.Minute)
	order := anOrder(merchant.StatusPlaced)
	order.AcceptDeadline = &deadline
	store := &stubStore{shop: anActiveShop(), orders: []merchant.Order{order}}

	response := call(t, serve(store, identity.RoleMerchant), http.MethodGet,
		"/api/v1/merchant/orders", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}

	var body struct {
		Items []merchant.OrderResponse `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %d", len(body.Items))
	}
	if body.Items[0].AcceptDeadline == nil {
		t.Error("the deadline is the most useful field in the New queue and it is absent")
	}
	if len(body.Items[0].Items) != 0 {
		t.Error("a queue of thirty orders must not be thirty-one queries")
	}
}

func TestTheDetailCarriesLinesAndTheirTotals(t *testing.T) {
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusConfirmed)}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodGet,
		"/api/v1/merchant/orders/order-1", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}

	var body merchant.OrderResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %+v", body.Items)
	}
	// Two at 600.00 is 1200.00. A picker reads the line total, not the unit
	// price times a number they work out themselves.
	if body.Items[0].LineTotal.Minor != 120000 {
		t.Errorf("line total = %d, want 120000", body.Items[0].LineTotal.Minor)
	}
	if body.Items[0].SubstitutionPreference != string(merchant.PreferAsk) {
		t.Errorf("preference = %q, want the customer's standing instruction",
			body.Items[0].SubstitutionPreference)
	}
}

func TestAnotherShopsOrderIsInvisible(t *testing.T) {
	// Not a forbidden: a merchant who can tell that an order id exists but
	// belongs to a competitor has learned something the platform did not mean
	// to tell them.
	elsewhere := anOrder(merchant.StatusPlaced)
	elsewhere.MerchantID = otherShop
	store := &stubStore{shop: anActiveShop(), order: elsewhere}

	for _, target := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/merchant/orders/order-1"},
		{http.MethodPost, "/api/v1/merchant/orders/order-1/accept"},
		{http.MethodPost, "/api/v1/merchant/orders/order-1/preparing"},
	} {
		response := call(t, serve(store, identity.RoleMerchant), target.method, target.path, "")
		if response.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", target.method, target.path, response.Code)
		}
	}
	if len(store.transitions) != 0 {
		t.Errorf("another shop's order was moved: %+v", store.transitions)
	}
}

func TestAcceptConfirmsAPlacedOrder(t *testing.T) {
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusPlaced)}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodPost,
		"/api/v1/merchant/orders/order-1/accept", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.transitions) != 1 {
		t.Fatalf("transitions = %+v", store.transitions)
	}
	moved := store.transitions[0]
	if moved.from != merchant.StatusPlaced || moved.to != merchant.StatusConfirmed {
		t.Errorf("moved %s → %s", moved.from, moved.to)
	}
	// Compare-and-set on the status the surface read, so an order the sweeper
	// cancelled a moment ago is not confirmed on top of the cancellation.
	if moved.actorID != shopID {
		t.Errorf("actor = %q, want the merchant", moved.actorID)
	}
}

func TestAcceptingTwiceIsNotAnError(t *testing.T) {
	// A merchant on a bad connection taps Accept twice. They answered once.
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusConfirmed)}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodPost,
		"/api/v1/merchant/orders/order-1/accept", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.transitions) != 0 {
		t.Errorf("the second tap moved the order again: %+v", store.transitions)
	}
}

func TestAnUnpaidOrderIsNotConfirmed(t *testing.T) {
	// Confirming commits the shop's stock. No payment surface exists to ask
	// whether the money arrived, so this refuses rather than assumes.
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusPaymentPending)}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodPost,
		"/api/v1/merchant/orders/order-1/accept", "")

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.transitions) != 0 {
		t.Errorf("an unpaid order was confirmed: %+v", store.transitions)
	}
}

func TestRejectionNeedsAReason(t *testing.T) {
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusPlaced)}
	mux := serve(store, identity.RoleMerchant)

	for _, body := range []string{"", `{}`, `{"reason":"   "}`} {
		response := call(t, mux, http.MethodPost, "/api/v1/merchant/orders/order-1/reject", body)
		if response.Code == http.StatusOK {
			t.Errorf("body %q was accepted; a customer cannot act on a rejection with no reason", body)
		}
	}
	if len(store.rejections) != 0 {
		t.Errorf("rejections = %+v", store.rejections)
	}
}

func TestRejectionRecordsTheReason(t *testing.T) {
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusPlaced)}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodPost,
		"/api/v1/merchant/orders/order-1/reject", `{"reason":"the milk delivery did not arrive"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.rejections) != 1 || store.rejections[0].reason != "the milk delivery did not arrive" {
		t.Fatalf("rejections = %+v", store.rejections)
	}
}

func TestAnOrderBeingPickedCannotBeRejected(t *testing.T) {
	// Document 070: rejection is reachable only before preparation. Once a
	// picker has walked the aisles, the resolution is operational.
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusPreparing)}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodPost,
		"/api/v1/merchant/orders/order-1/reject", `{"reason":"changed my mind"}`)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.rejections) != 0 {
		t.Errorf("rejections = %+v", store.rejections)
	}
}

func TestPreparingRequiresAcceptanceFirst(t *testing.T) {
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusPlaced)}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodPost,
		"/api/v1/merchant/orders/order-1/preparing", "")

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.transitions) != 0 {
		t.Errorf("transitions = %+v", store.transitions)
	}
}

func TestPreparingStartsPicking(t *testing.T) {
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusConfirmed)}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodPost,
		"/api/v1/merchant/orders/order-1/preparing", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.transitions) != 1 || store.transitions[0].to != merchant.StatusPreparing {
		t.Fatalf("transitions = %+v", store.transitions)
	}
}

func TestASuspendedMerchantCanLookButNotAct(t *testing.T) {
	suspended := anActiveShop()
	suspended.Status = "SUSPENDED"
	store := &stubStore{shop: suspended, order: anOrder(merchant.StatusPlaced)}
	mux := serve(store, identity.RoleMerchant)

	if response := call(t, mux, http.MethodGet, "/api/v1/merchant/orders", ""); response.Code != http.StatusOK {
		t.Errorf("reading the queue = %d; a suspended merchant still needs to see what happened",
			response.Code)
	}
	response := call(t, mux, http.MethodPost, "/api/v1/merchant/orders/order-1/accept", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("accept = %d, want 403", response.Code)
	}
	if len(store.transitions) != 0 {
		t.Errorf("a suspended merchant accepted an order: %+v", store.transitions)
	}
}

func TestTheRoleAloneIsNotEnough(t *testing.T) {
	// The role says a merchant is calling. It does not say which one, and an
	// account that holds it before onboarding finished operates no shop.
	store := &stubStore{shopErr: merchant.ErrNotFound}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodGet,
		"/api/v1/merchant/orders", "")

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
}

func TestTwoShopsUnderOneAccountAreRefusedRatherThanGuessed(t *testing.T) {
	// Showing one shop's queue to an owner who meant the other is a merchant
	// accepting an order they cannot fulfil.
	store := &stubStore{shopErr: merchant.ErrManyMerchants}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodGet,
		"/api/v1/merchant/orders", "")

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
}

func TestACustomerCannotReachTheMerchantSurface(t *testing.T) {
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusPlaced)}
	response := call(t, serve(store, identity.RoleCustomer), http.MethodPost,
		"/api/v1/merchant/orders/order-1/accept", "")

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
}

func TestTheCustomerIsNotNamedToTheShop(t *testing.T) {
	// A shop needs to know what to pick and by when. Who ordered it is not
	// theirs, and a field nobody needs is a field that leaks.
	store := &stubStore{shop: anActiveShop(), order: anOrder(merchant.StatusConfirmed)}
	response := call(t, serve(store, identity.RoleMerchant), http.MethodGet,
		"/api/v1/merchant/orders/order-1", "")

	if strings.Contains(response.Body.String(), "customer-9") {
		t.Errorf("the customer's identity reached the merchant: %s", response.Body)
	}
}
