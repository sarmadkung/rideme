package merchant

import (
	"context"
	"errors"
	"strings"
	"time"
)

// DefaultSearchRadiusM is how far a customer's "near me" reaches.
//
// Five kilometres is a delivery, not a catchment area: a grocery order from
// further away arrives warm in Lahore traffic. It is an engineering default,
// not a decided service radius — zones (document 097) are where that belongs
// and they are not built.
const DefaultSearchRadiusM = 5000

// MaxSearchRadiusM bounds what a caller can widen it to.
const MaxSearchRadiusM = 25000

// MaxLineQuantity bounds one cart line. Somebody ordering four hundred litres
// of milk is a mistake or an attack, and either way it is not a grocery order.
const MaxLineQuantity = 100

// CatalogStore is what the customer surface needs from persistence.
type CatalogStore interface {
	OutletsNear(ctx context.Context, lat, lon, radiusM float64, at time.Time, limit int) ([]Outlet, error)
	OutletByID(ctx context.Context, id string) (Outlet, error)
	ProductsAt(ctx context.Context, storeID string, limit int) ([]Product, error)
	OpenCart(ctx context.Context, merchantID, storeID, customerID string) (Order, error)
	AddItem(ctx context.Context, orderID, productID, variantID string, quantity int,
		preference SubstitutionPreference) (Item, error)
	OrderByID(ctx context.Context, id string) (Order, error)
	OrdersOf(ctx context.Context, customerUserID string, limit int) ([]Order, error)
	Place(ctx context.Context, orderID string, to Delivery, now time.Time) (Order, error)
	IssuesOf(ctx context.Context, orderID string) ([]Issue, error)
	SettleIssue(ctx context.Context, orderID, issueID, resolution, itemStatus string) (Issue, error)
}

// CustomerService is the customer's side of a grocery order (documents 068, 071).
type CustomerService struct {
	store CatalogStore
	now   func() time.Time
}

func NewCustomerService(store CatalogStore) *CustomerService {
	return &CustomerService{store: store, now: time.Now}
}

// WithClock replaces the clock, for tests that assert on opening hours.
func (s *CustomerService) WithClock(now func() time.Time) *CustomerService {
	s.now = now
	return s
}

// Outlets lists shops a customer can order from.
func (s *CustomerService) Outlets(ctx context.Context, lat, lon, radiusM float64,
	limit int) ([]Outlet, error) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, ErrBadPoint
	}
	if radiusM <= 0 {
		radiusM = DefaultSearchRadiusM
	}
	if radiusM > MaxSearchRadiusM {
		radiusM = MaxSearchRadiusM
	}
	return s.store.OutletsNear(ctx, lat, lon, radiusM, s.now(), limit)
}

// Catalog lists what one shop has.
func (s *CustomerService) Catalog(ctx context.Context, storeID string, limit int) ([]Product, error) {
	if _, err := s.store.OutletByID(ctx, storeID); err != nil {
		return nil, err
	}
	return s.store.ProductsAt(ctx, storeID, limit)
}

// OpenCart returns the customer's cart at a shop, creating one if needed.
//
// The merchant is resolved from the shop rather than taken from the client. A
// customer choosing a branch has chosen its owner, and a client that could
// name both could name a mismatched pair.
func (s *CustomerService) OpenCart(ctx context.Context, userID, storeID string) (Order, error) {
	outlet, err := s.store.OutletByID(ctx, storeID)
	if err != nil {
		return Order{}, err
	}
	return s.store.OpenCart(ctx, outlet.MerchantID, outlet.ID, userID)
}

// AddItem adds a line to the customer's own cart.
func (s *CustomerService) AddItem(ctx context.Context, userID, orderID, productID, variantID string,
	quantity int, preference SubstitutionPreference) (Item, error) {
	if quantity <= 0 || quantity > MaxLineQuantity {
		return Item{}, ErrBadQuantity
	}
	if preference != "" && preference != PreferAllow &&
		preference != PreferDoNotAllow && preference != PreferAsk {
		return Item{}, ErrBadPreference
	}
	order, err := s.ownCart(ctx, userID, orderID)
	if err != nil {
		return Item{}, err
	}
	return s.store.AddItem(ctx, order.ID, productID, variantID, quantity, preference)
}

// Order loads one of the customer's own orders.
func (s *CustomerService) Order(ctx context.Context, userID, orderID string) (Order, error) {
	order, err := s.store.OrderByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	if order.CustomerUserID != userID {
		// Not a forbidden: whether an order id exists is not a customer's
		// business either.
		return Order{}, ErrNotFound
	}
	return order, nil
}

// Orders lists the customer's own order history.
func (s *CustomerService) Orders(ctx context.Context, userID string, limit int) ([]Order, error) {
	return s.store.OrdersOf(ctx, userID, limit)
}

// Place checks out the cart to a destination (document 071's Address → Place).
func (s *CustomerService) Place(ctx context.Context, userID, orderID string,
	to Delivery) (Order, error) {
	to.Address = strings.TrimSpace(to.Address)
	to.Notes = strings.TrimSpace(to.Notes)
	if !to.Valid() {
		return Order{}, ErrNoDestination
	}
	order, err := s.ownCart(ctx, userID, orderID)
	if err != nil {
		return Order{}, err
	}
	return s.store.Place(ctx, order.ID, to, s.now())
}

// DecideIssue records the customer's answer to a proposed substitution
// (document 074).
//
// Only the customer answers, and only on their own order: the whole point of
// ASK_ME is that the shop does not get to decide.
func (s *CustomerService) DecideIssue(ctx context.Context, userID, orderID, issueID string,
	accept bool) (Issue, error) {
	order, err := s.Order(ctx, userID, orderID)
	if err != nil {
		return Issue{}, err
	}
	if accept {
		// BD-11: the customer pays the substitute's actual price, up or down.
		return s.store.SettleIssue(ctx, order.ID, issueID,
			ResolutionCustomerAccepted, ItemSubstituted)
	}
	// Declined means the line is gone, not that the original appears — the
	// shelf is empty, which is why they were asked at all.
	return s.store.SettleIssue(ctx, order.ID, issueID, ResolutionCustomerDeclined, ItemRemoved)
}

// Issues lists what the shop found missing on an order.
func (s *CustomerService) Issues(ctx context.Context, orderID string) ([]Issue, error) {
	return s.store.IssuesOf(ctx, orderID)
}

// ownCart resolves an order the caller owns and that is still a cart.
//
// A placed order is not editable by the customer: the merchant may already be
// picking it, and a line added after acceptance is a line nobody agreed to.
func (s *CustomerService) ownCart(ctx context.Context, userID, orderID string) (Order, error) {
	order, err := s.Order(ctx, userID, orderID)
	if err != nil {
		return Order{}, err
	}
	if order.Status != StatusCart {
		return Order{}, ErrNotACart
	}
	return order, nil
}

var (
	// ErrBadPoint reports coordinates outside the world.
	ErrBadPoint = errors.New("merchant: that is not a position on Earth")
	// ErrBadQuantity reports a line quantity outside what a grocery order is.
	ErrBadQuantity = errors.New("merchant: that is not a quantity")
	// ErrBadPreference reports a substitution preference outside document 074's three.
	ErrBadPreference = errors.New("merchant: no such substitution preference")
	// ErrNotACart reports an edit to an order the customer has already placed.
	ErrNotACart = errors.New("merchant: this order has been placed and can no longer be changed")
)
