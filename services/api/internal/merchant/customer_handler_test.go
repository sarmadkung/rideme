package merchant_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

const customerID = "customer-1"

// stubCatalog stands in for Postgres on the customer side.
type stubCatalog struct {
	outlets    []merchant.Outlet
	outlet     merchant.Outlet
	outletErr  error
	products   []merchant.Product
	cart       merchant.Order
	order      merchant.Order
	orderErr   error
	addErr     error
	placeErr   error
	settleErr  error
	openIssues []merchant.Issue
	decisions  []decision

	cancellations []cancellation

	askedLat, askedLon, askedRadius float64
	askedAt                         time.Time
	openedMerchant, openedStore     string
	added                           []addedLine
	placed                          []merchant.Delivery
}

type decision struct{ orderID, issueID, resolution, itemStatus string }

type cancellation struct {
	orderID string
	from    merchant.OrderStatus
	reason  string
}

type addedLine struct {
	orderID, productID, variantID string
	quantity                      int
	preference                    merchant.SubstitutionPreference
}

func (s *stubCatalog) OutletsNear(_ context.Context, lat, lon, radiusM float64,
	at time.Time, _ int) ([]merchant.Outlet, error) {
	s.askedLat, s.askedLon, s.askedRadius, s.askedAt = lat, lon, radiusM, at
	return s.outlets, nil
}

func (s *stubCatalog) OutletByID(_ context.Context, _ string) (merchant.Outlet, error) {
	if s.outletErr != nil {
		return merchant.Outlet{}, s.outletErr
	}
	return s.outlet, nil
}

func (s *stubCatalog) ProductsAt(_ context.Context, _ string, _ int) ([]merchant.Product, error) {
	return s.products, nil
}

func (s *stubCatalog) OpenCart(_ context.Context, merchantID, storeID, _ string) (merchant.Order, error) {
	s.openedMerchant, s.openedStore = merchantID, storeID
	return s.cart, nil
}

func (s *stubCatalog) AddItem(_ context.Context, orderID, productID, variantID string,
	quantity int, preference merchant.SubstitutionPreference) (merchant.Item, error) {
	if s.addErr != nil {
		return merchant.Item{}, s.addErr
	}
	s.added = append(s.added, addedLine{orderID, productID, variantID, quantity, preference})
	return merchant.Item{ID: "item-new"}, nil
}

func (s *stubCatalog) OrderByID(_ context.Context, _ string) (merchant.Order, error) {
	if s.orderErr != nil {
		return merchant.Order{}, s.orderErr
	}
	return s.order, nil
}

func (s *stubCatalog) OrdersOf(_ context.Context, _ string, _ int) ([]merchant.Order, error) {
	return []merchant.Order{s.order}, nil
}

func (s *stubCatalog) CancelByCustomer(_ context.Context, orderID string,
	from merchant.OrderStatus, reason string) (merchant.Order, error) {
	s.cancellations = append(s.cancellations, cancellation{orderID, from, reason})
	cancelled := s.order
	cancelled.Status = merchant.StatusCancelled
	return cancelled, nil
}

func (s *stubCatalog) IssuesOf(_ context.Context, _ string) ([]merchant.Issue, error) {
	return s.openIssues, nil
}

func (s *stubCatalog) SettleIssue(_ context.Context, orderID, issueID, resolution,
	itemStatus string) (merchant.Issue, error) {
	if s.settleErr != nil {
		return merchant.Issue{}, s.settleErr
	}
	s.decisions = append(s.decisions, decision{orderID, issueID, resolution, itemStatus})
	return merchant.Issue{ID: issueID, Resolution: resolution}, nil
}

func (s *stubCatalog) Place(_ context.Context, _ string, to merchant.Delivery,
	_ time.Time) (merchant.Order, error) {
	if s.placeErr != nil {
		return merchant.Order{}, s.placeErr
	}
	s.placed = append(s.placed, to)
	placed := s.order
	placed.Status = merchant.StatusPlaced
	placed.Delivery = to
	return placed, nil
}

func aCart() merchant.Order {
	return merchant.Order{
		ID:             "order-1",
		MerchantID:     shopID,
		StoreID:        "store-1",
		CustomerUserID: customerID,
		Status:         merchant.StatusCart,
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

func serveCustomer(store merchant.CatalogStore) *http.ServeMux {
	return serveCustomerAt(store, time.Now())
}

func serveCustomerAt(store merchant.CatalogStore, at time.Time) *http.ServeMux {
	mux := http.NewServeMux()
	service := merchant.NewCustomerService(store).WithClock(func() time.Time { return at })
	merchant.NewCustomerHandler(service).Routes(mux,
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := identity.ContextWithPrincipal(r.Context(), identity.Principal{
					UserID: customerID, Roles: []identity.Role{identity.RoleCustomer},
				})
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
	return mux
}

func TestNearbyShopsNeedAPosition(t *testing.T) {
	// Without one there is no meaningful answer: "every shop in Pakistan" is
	// not a list a customer can use.
	response := call(t, serveCustomer(&stubCatalog{}), http.MethodGet, "/api/v1/stores", "")
	if response.Code == http.StatusOK {
		t.Fatalf("status = %d, want a refusal", response.Code)
	}
}

func TestAPositionOffTheEarthIsRefused(t *testing.T) {
	store := &stubCatalog{}
	response := call(t, serveCustomer(store), http.MethodGet,
		"/api/v1/stores?lat=91&lon=74.3", "")
	if response.Code == http.StatusOK {
		t.Fatalf("status = %d, want a refusal", response.Code)
	}
	if store.askedRadius != 0 {
		t.Error("the query reached the database")
	}
}

func TestTheSearchRadiusIsDefaultedAndCapped(t *testing.T) {
	for _, testCase := range []struct {
		asked string
		want  float64
	}{
		{"", merchant.DefaultSearchRadiusM},
		{"&radius_m=1200", 1200},
		{"&radius_m=400000", merchant.MaxSearchRadiusM},
	} {
		store := &stubCatalog{}
		call(t, serveCustomer(store), http.MethodGet,
			"/api/v1/stores?lat=31.52&lon=74.35"+testCase.asked, "")
		if store.askedRadius != testCase.want {
			t.Errorf("radius for %q = %v, want %v", testCase.asked, store.askedRadius, testCase.want)
		}
	}
}

func TestAClosedShopIsListedAsClosedRatherThanHidden(t *testing.T) {
	// A customer looking for their usual kiryana at 3am needs to see that it
	// is shut, not that it has disappeared.
	store := &stubCatalog{outlets: []merchant.Outlet{{
		ID: "store-1", MerchantName: "Al-Fatah", Name: "Gulberg",
		Lat: 31.5169, Lon: 74.3484, DistanceM: 420, Open: false,
	}}}
	response := call(t, serveCustomer(store), http.MethodGet,
		"/api/v1/stores?lat=31.52&lon=74.35", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}

	var body struct {
		Stores []merchant.OutletResponse `json:"stores"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Stores) != 1 {
		t.Fatalf("stores = %d", len(body.Stores))
	}
	if body.Stores[0].Open {
		t.Error("a closed shop was reported open")
	}
	if body.Stores[0].DistanceM != 420 {
		t.Errorf("distance = %v; a client should not have to recompute it", body.Stores[0].DistanceM)
	}
}

func TestTheCatalogueRefusesAnUnknownShop(t *testing.T) {
	store := &stubCatalog{outletErr: merchant.ErrNotFound}
	response := call(t, serveCustomer(store), http.MethodGet,
		"/api/v1/stores/nope/products", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
}

func TestTheCatalogueCarriesVariantsAndAvailability(t *testing.T) {
	store := &stubCatalog{
		outlet: merchant.Outlet{ID: "store-1", MerchantID: shopID},
		products: []merchant.Product{{
			ID: "p1", Name: "Milk", Price: money.MustNew(25000, money.PKR), Available: true,
			Variants: []merchant.Variant{
				{ID: "v1", Name: "1L", Delta: money.MustNew(0, money.PKR), Available: true},
				{ID: "v2", Name: "2L", Delta: money.MustNew(20000, money.PKR), Available: false},
			},
		}},
	}
	response := call(t, serveCustomer(store), http.MethodGet,
		"/api/v1/stores/store-1/products", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}

	var body struct {
		Products []merchant.ProductResponse `json:"products"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Products) != 1 || len(body.Products[0].Variants) != 2 {
		t.Fatalf("products = %+v", body.Products)
	}
	// The 2L is out of stock at this branch and says so, rather than being
	// absent and looking like it does not exist.
	if body.Products[0].Variants[1].Available {
		t.Error("an out-of-stock variant was reported available")
	}
	if body.Products[0].Variants[1].PriceDiff.Minor != 20000 {
		t.Errorf("price difference = %d", body.Products[0].Variants[1].PriceDiff.Minor)
	}
}

func TestTheCartsMerchantComesFromTheShopNotTheClient(t *testing.T) {
	// A client that could name both could name a mismatched pair, and an
	// order under the wrong merchant is one the right merchant never sees.
	store := &stubCatalog{
		outlet: merchant.Outlet{ID: "store-7", MerchantID: "merchant-7"},
		cart:   aCart(),
	}
	response := call(t, serveCustomer(store), http.MethodPost, "/api/v1/orders",
		`{"store_id":"store-7","merchant_id":"merchant-attacker"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if store.openedMerchant != "merchant-7" || store.openedStore != "store-7" {
		t.Fatalf("opened %s/%s, want merchant-7/store-7", store.openedMerchant, store.openedStore)
	}
}

func TestAddingALineAnswersWithTheWholeCart(t *testing.T) {
	// A client showing a running total needs the total the server computed;
	// asking for it separately is a round trip and a chance to disagree.
	store := &stubCatalog{order: aCart()}
	response := call(t, serveCustomer(store), http.MethodPost,
		"/api/v1/orders/order-1/items", `{"product_id":"p1","quantity":2}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}

	var body merchant.CartResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ItemsTotal.Minor != 120000 || len(body.Items) != 1 {
		t.Fatalf("cart = %+v", body)
	}
	if len(store.added) != 1 || store.added[0].quantity != 2 {
		t.Fatalf("added = %+v", store.added)
	}
}

func TestAnAbsurdQuantityIsRefused(t *testing.T) {
	store := &stubCatalog{order: aCart()}
	mux := serveCustomer(store)
	for _, body := range []string{
		`{"product_id":"p1","quantity":0}`,
		`{"product_id":"p1","quantity":-3}`,
		`{"product_id":"p1","quantity":4000}`,
	} {
		response := call(t, mux, http.MethodPost, "/api/v1/orders/order-1/items", body)
		if response.Code == http.StatusOK {
			t.Errorf("%s was accepted", body)
		}
	}
	if len(store.added) != 0 {
		t.Errorf("added = %+v", store.added)
	}
}

func TestAnUnknownSubstitutionPreferenceIsRefused(t *testing.T) {
	// Document 074 has three. A fourth would be stored and then interpreted by
	// ResolveIssue's default, which asks — quietly turning a typo into a rule.
	store := &stubCatalog{order: aCart()}
	response := call(t, serveCustomer(store), http.MethodPost,
		"/api/v1/orders/order-1/items",
		`{"product_id":"p1","quantity":1,"substitution_preference":"WHATEVER"}`)
	if response.Code == http.StatusOK {
		t.Fatalf("status = %d, want a refusal", response.Code)
	}
	if len(store.added) != 0 {
		t.Errorf("added = %+v", store.added)
	}
}

func TestSomebodyElsesCartIsInvisible(t *testing.T) {
	theirs := aCart()
	theirs.CustomerUserID = "someone-else"
	store := &stubCatalog{order: theirs}
	mux := serveCustomer(store)

	for _, target := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/orders/order-1", ""},
		{http.MethodPost, "/api/v1/orders/order-1/items", `{"product_id":"p1","quantity":1}`},
		{http.MethodPost, "/api/v1/orders/order-1/place",
			`{"delivery":{"address":"somewhere","latitude":31.5,"longitude":74.3}}`},
	} {
		response := call(t, mux, target.method, target.path, target.body)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", target.method, target.path, response.Code)
		}
	}
	if len(store.added) != 0 || len(store.placed) != 0 {
		t.Error("somebody else's cart was modified")
	}
}

func TestAPlacedOrderCanNoLongerBeChanged(t *testing.T) {
	// The merchant may already be picking it, and a line added after
	// acceptance is a line nobody agreed to.
	placed := aCart()
	placed.Status = merchant.StatusConfirmed
	store := &stubCatalog{order: placed}
	response := call(t, serveCustomer(store), http.MethodPost,
		"/api/v1/orders/order-1/items", `{"product_id":"p1","quantity":1}`)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.added) != 0 {
		t.Errorf("added = %+v", store.added)
	}
}

func TestCheckoutRequiresSomewhereToDeliver(t *testing.T) {
	// Document 071 puts Address before Place Order. Half a destination is
	// worse than none: an address with no coordinates cannot be routed to and
	// coordinates with no address cannot be read out to a driver.
	store := &stubCatalog{order: aCart()}
	mux := serveCustomer(store)

	for _, body := range []string{
		`{}`,
		`{"delivery":{}}`,
		`{"delivery":{"address":"House 12, Gulberg"}}`,
		`{"delivery":{"latitude":31.5169,"longitude":74.3484}}`,
		`{"delivery":{"address":"   ","latitude":31.5169,"longitude":74.3484}}`,
		`{"delivery":{"address":"House 12","latitude":0,"longitude":0}}`,
	} {
		response := call(t, mux, http.MethodPost, "/api/v1/orders/order-1/place", body)
		if response.Code == http.StatusOK {
			t.Errorf("%s was placed", body)
		}
	}
	if len(store.placed) != 0 {
		t.Errorf("placed = %+v", store.placed)
	}
}

func TestCheckoutRecordsTheDestination(t *testing.T) {
	store := &stubCatalog{order: aCart()}
	response := call(t, serveCustomer(store), http.MethodPost, "/api/v1/orders/order-1/place",
		`{"delivery":{"address":" House 12, Gulberg III ","latitude":31.5169,`+
			`"longitude":74.3484,"notes":" second gate "}}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.placed) != 1 {
		t.Fatalf("placed = %+v", store.placed)
	}
	if store.placed[0].Address != "House 12, Gulberg III" {
		t.Errorf("address = %q, want it trimmed", store.placed[0].Address)
	}
	if store.placed[0].Notes != "second gate" {
		t.Errorf("notes = %q", store.placed[0].Notes)
	}

	var body merchant.CartResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Delivery == nil || body.Delivery.Latitude != 31.5169 {
		t.Fatalf("the placed order does not carry its destination: %+v", body.Delivery)
	}
}

func TestACartCarriesNoDestinationUntilCheckout(t *testing.T) {
	store := &stubCatalog{order: aCart()}
	response := call(t, serveCustomer(store), http.MethodGet, "/api/v1/orders/order-1", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}

	var body merchant.CartResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Delivery != nil {
		t.Errorf("delivery = %+v, want absent until it is asked for", body.Delivery)
	}
}

func TestAnUnavailableItemIsAConflictNotAMissingOne(t *testing.T) {
	// "Not found" sends a customer looking for a typo. Out of stock tells them
	// to pick something else.
	store := &stubCatalog{order: aCart(), addErr: merchant.ErrOutOfStock}
	response := call(t, serveCustomer(store), http.MethodPost,
		"/api/v1/orders/order-1/items", `{"product_id":"p1","quantity":1}`)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
}

func TestAnEmptyCartIsNotAnInternalFailure(t *testing.T) {
	store := &stubCatalog{order: aCart(), placeErr: merchant.ErrEmptyCart}
	response := call(t, serveCustomer(store), http.MethodPost, "/api/v1/orders/order-1/place",
		`{"delivery":{"address":"House 12","latitude":31.5169,"longitude":74.3484}}`)

	if response.Code >= 500 {
		t.Fatalf("status = %d; an empty cart is the customer's mistake, not the server's",
			response.Code)
	}
}

// --- answering a substitution (document 074, BD-11) -------------------------

func aPendingIssue() merchant.Issue {
	price := money.MustNew(62000, money.PKR)
	difference := money.MustNew(4000, money.PKR)
	return merchant.Issue{
		ID: "issue-1", OrderID: "order-1", OrderItemID: "item-1",
		Reason: "shelf empty", Action: merchant.ActionAsk,
		SubstituteName: "Nurpur 1L", SubstitutePrice: &price, PriceDifference: &difference,
		Resolution: merchant.ResolutionPending,
	}
}

func TestAPendingSubstitutionTravelsWithTheOrder(t *testing.T) {
	// A question waiting on this customer has to reach the screen they are
	// looking at, not sit behind a request they would have to know to make.
	store := &stubCatalog{order: aCart(), openIssues: []merchant.Issue{aPendingIssue()}}
	response := call(t, serveCustomer(store), http.MethodGet, "/api/v1/orders/order-1", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	var body merchant.CartResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Issues) != 1 {
		t.Fatalf("issues = %+v", body.Issues)
	}
	if body.Issues[0].SubstituteName != "Nurpur 1L" || body.Issues[0].SubstitutePrice == nil {
		t.Errorf("the customer was asked about nothing in particular: %+v", body.Issues[0])
	}
	if body.Issues[0].PriceDifference == nil || body.Issues[0].PriceDifference.Minor != 4000 {
		t.Errorf("price difference = %+v; BD-11 sends it to the customer",
			body.Issues[0].PriceDifference)
	}
}

func TestAcceptingASubstituteTakesItAtItsPrice(t *testing.T) {
	store := &stubCatalog{order: aCart()}
	response := call(t, serveCustomer(store), http.MethodPost,
		"/api/v1/orders/order-1/issues/issue-1/decision", `{"accept":true}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.decisions) != 1 {
		t.Fatalf("decisions = %+v", store.decisions)
	}
	if store.decisions[0].resolution != merchant.ResolutionCustomerAccepted {
		t.Errorf("resolution = %s", store.decisions[0].resolution)
	}
	if store.decisions[0].itemStatus != merchant.ItemSubstituted {
		t.Errorf("item status = %s", store.decisions[0].itemStatus)
	}
}

func TestDecliningLosesTheLineRatherThanRestoringIt(t *testing.T) {
	// The shelf is empty — that is why they were asked. Declining cannot put
	// the original in the bag.
	store := &stubCatalog{order: aCart()}
	response := call(t, serveCustomer(store), http.MethodPost,
		"/api/v1/orders/order-1/issues/issue-1/decision", `{"accept":false}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if len(store.decisions) != 1 || store.decisions[0].itemStatus != merchant.ItemRemoved {
		t.Fatalf("decisions = %+v", store.decisions)
	}
	if store.decisions[0].resolution != merchant.ResolutionCustomerDeclined {
		t.Errorf("resolution = %s", store.decisions[0].resolution)
	}
}

func TestAnAnswerToSomebodyElsesOrderIsRefused(t *testing.T) {
	theirs := aCart()
	theirs.CustomerUserID = "someone-else"
	store := &stubCatalog{order: theirs}
	response := call(t, serveCustomer(store), http.MethodPost,
		"/api/v1/orders/order-1/issues/issue-1/decision", `{"accept":true}`)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if len(store.decisions) != 0 {
		t.Errorf("somebody else answered for this customer: %+v", store.decisions)
	}
}

func TestAnsweringTwiceIsRefusedRatherThanRepriced(t *testing.T) {
	// The merchant may have given up and removed the item while the customer
	// was thinking. Whoever got there first decided.
	store := &stubCatalog{order: aCart(), settleErr: merchant.ErrIssueSettled}
	response := call(t, serveCustomer(store), http.MethodPost,
		"/api/v1/orders/order-1/issues/issue-1/decision", `{"accept":true}`)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", response.Code)
	}
}

// --- cancelling your own order (document 070) --------------------------------

func TestACustomerCanCallOffAnOrderNobodyHasPicked(t *testing.T) {
	for _, status := range []merchant.OrderStatus{
		merchant.StatusCart, merchant.StatusPlaced,
		merchant.StatusPaymentPending, merchant.StatusConfirmed,
	} {
		order := aCart()
		order.Status = status
		store := &stubCatalog{order: order}
		response := call(t, serveCustomer(store), http.MethodPost,
			"/api/v1/orders/order-1/cancel", `{"reason":"ordered by mistake"}`)

		if response.Code != http.StatusOK {
			t.Errorf("%s: status = %d, body %s", status, response.Code, response.Body)
			continue
		}
		if len(store.cancellations) != 1 || store.cancellations[0].from != status {
			t.Errorf("%s: cancellations = %+v", status, store.cancellations)
		}
	}
}

func TestCancellingNeedsNoReason(t *testing.T) {
	// A customer who changed their mind owes nobody an explanation, and a
	// required field would only collect "asdf".
	store := &stubCatalog{order: aCart()}
	mux := serveCustomer(store)

	for _, body := range []string{"", `{}`, `{"reason":""}`} {
		response := call(t, mux, http.MethodPost, "/api/v1/orders/order-1/cancel", body)
		if response.Code != http.StatusOK {
			t.Errorf("body %q: status = %d, body %s", body, response.Code, response.Body)
		}
	}
	if len(store.cancellations) != 3 {
		t.Errorf("cancellations = %+v", store.cancellations)
	}
}

func TestAnOrderBeingPickedCannotBeCalledOff(t *testing.T) {
	// Document 070: once a picker has walked the aisles, cancelling wastes
	// goods somebody has handled. The answer says who can help instead.
	for _, status := range []merchant.OrderStatus{
		merchant.StatusPreparing, merchant.StatusReadyForPickup,
		merchant.StatusPickedUp, merchant.StatusDelivering, merchant.StatusDelivered,
	} {
		order := aCart()
		order.Status = status
		store := &stubCatalog{order: order}
		response := call(t, serveCustomer(store), http.MethodPost,
			"/api/v1/orders/order-1/cancel", "")

		if response.Code != http.StatusConflict {
			t.Errorf("%s: status = %d, want 409", status, response.Code)
		}
		if len(store.cancellations) != 0 {
			t.Errorf("%s: an order being prepared was cancelled: %+v", status, store.cancellations)
		}
	}
}

func TestSomebodyElsesOrderCannotBeCancelled(t *testing.T) {
	theirs := aCart()
	theirs.CustomerUserID = "someone-else"
	store := &stubCatalog{order: theirs}
	response := call(t, serveCustomer(store), http.MethodPost,
		"/api/v1/orders/order-1/cancel", "")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if len(store.cancellations) != 0 {
		t.Errorf("cancellations = %+v", store.cancellations)
	}
}
