package merchant

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// CustomerHandler serves the customer's side of a grocery order — the shops,
// the catalogue, the cart and checkout (documents 068, 071).
//
// Authenticated but not role-gated, like the ride surface: every caller is
// somebody's customer, and each route is scoped to the caller's own orders.
type CustomerHandler struct{ service *CustomerService }

func NewCustomerHandler(service *CustomerService) *CustomerHandler {
	return &CustomerHandler{service: service}
}

func (h *CustomerHandler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	auth := func(fn http.HandlerFunc) http.Handler { return authenticate(fn) }

	mux.Handle("GET "+p+"/stores", auth(h.outlets))
	mux.Handle("GET "+p+"/stores/{id}/products", auth(h.catalog))
	mux.Handle("POST "+p+"/orders", auth(h.openCart))
	mux.Handle("GET "+p+"/orders", auth(h.orders))
	mux.Handle("GET "+p+"/orders/{id}", auth(h.order))
	mux.Handle("POST "+p+"/orders/{id}/items", auth(h.addItem))
	mux.Handle("POST "+p+"/orders/{id}/place", auth(h.place))
}

// --- responses ---------------------------------------------------------------

// OutletResponse is a shop a customer can order from.
type OutletResponse struct {
	ID           string  `json:"id"`
	MerchantName string  `json:"merchant_name"`
	Name         string  `json:"name"`
	Address      string  `json:"address,omitempty"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	DistanceM    float64 `json:"distance_m"`
	// Open applies the store's hours to now. A shut shop is still listed —
	// a customer looking for their usual kiryana at 3am needs to see that it
	// is closed, not that it has vanished.
	Open bool `json:"open"`
}

// VariantResponse is a size or option and what it adds to the price.
type VariantResponse struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	PriceDiff money.Amount `json:"price_diff"`
	Available bool         `json:"available"`
}

// ProductResponse is a catalogue line at one store.
type ProductResponse struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Price       money.Amount      `json:"price"`
	Available   bool              `json:"available"`
	Variants    []VariantResponse `json:"variants,omitempty"`
}

// CartResponse is the customer's own view of an order.
type CartResponse struct {
	ID         string       `json:"id"`
	StoreID    string       `json:"store_id,omitempty"`
	Status     string       `json:"status"`
	ItemsTotal money.Amount `json:"items_total"`
	Items      []CartLine   `json:"items"`
	// Delivery is absent until checkout, which is when it is asked for.
	Delivery       *DeliveryResponse `json:"delivery,omitempty"`
	AcceptDeadline *time.Time        `json:"accept_deadline,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

// CartLine is one line as the customer sees it.
type CartLine struct {
	ID                     string       `json:"id"`
	Name                   string       `json:"name"`
	Quantity               int          `json:"quantity"`
	UnitPrice              money.Amount `json:"unit_price"`
	LineTotal              money.Amount `json:"line_total"`
	SubstitutionPreference string       `json:"substitution_preference"`
	Status                 string       `json:"status"`
}

// DeliveryResponse is where the order is going.
type DeliveryResponse struct {
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Notes     string  `json:"notes,omitempty"`
}

func toCartResponse(order Order) (CartResponse, error) {
	out := CartResponse{
		ID:             order.ID,
		StoreID:        order.StoreID,
		Status:         string(order.Status),
		ItemsTotal:     order.ItemsTotal,
		AcceptDeadline: order.AcceptDeadline,
		CreatedAt:      order.CreatedAt,
		Items:          make([]CartLine, 0, len(order.Items)),
	}
	if order.Delivery.Address != "" {
		out.Delivery = &DeliveryResponse{
			Address:   order.Delivery.Address,
			Latitude:  order.Delivery.Lat,
			Longitude: order.Delivery.Lon,
			Notes:     order.Delivery.Notes,
		}
	}
	for _, item := range order.Items {
		line, err := item.LineTotal()
		if err != nil {
			return CartResponse{}, err
		}
		out.Items = append(out.Items, CartLine{
			ID:                     item.ID,
			Name:                   item.NameSnapshot,
			Quantity:               item.Quantity,
			UnitPrice:              item.UnitPrice,
			LineTotal:              line,
			SubstitutionPreference: string(item.Preference),
			Status:                 item.Status,
		})
	}
	return out, nil
}

// --- handlers ----------------------------------------------------------------

func (h *CustomerHandler) outlets(w http.ResponseWriter, r *http.Request) {
	lat, latErr := floatParam(r, "lat")
	lon, lonErr := floatParam(r, "lon")
	if latErr != nil || lonErr != nil {
		httpx.WriteError(w, r, httpx.Validation("a position is required to find shops nearby",
			map[string]string{"lat": "required", "lon": "required"}))
		return
	}
	radius, _ := floatParam(r, "radius_m")

	found, err := h.service.Outlets(r.Context(), lat, lon, radius, intParam(r, "limit"))
	if err != nil {
		customerError(w, r, err)
		return
	}

	items := make([]OutletResponse, 0, len(found))
	for _, outlet := range found {
		items = append(items, OutletResponse{
			ID:           outlet.ID,
			MerchantName: outlet.MerchantName,
			Name:         outlet.Name,
			Address:      outlet.Address,
			Latitude:     outlet.Lat,
			Longitude:    outlet.Lon,
			DistanceM:    outlet.DistanceM,
			Open:         outlet.Open,
		})
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"stores": items})
}

func (h *CustomerHandler) catalog(w http.ResponseWriter, r *http.Request) {
	products, err := h.service.Catalog(r.Context(), r.PathValue("id"), intParam(r, "limit"))
	if err != nil {
		customerError(w, r, err)
		return
	}

	items := make([]ProductResponse, 0, len(products))
	for _, product := range products {
		line := ProductResponse{
			ID:          product.ID,
			Name:        product.Name,
			Description: product.Description,
			Price:       product.Price,
			Available:   product.Available,
		}
		for _, variant := range product.Variants {
			line.Variants = append(line.Variants, VariantResponse{
				ID:        variant.ID,
				Name:      variant.Name,
				PriceDiff: variant.Delta,
				Available: variant.Available,
			})
		}
		items = append(items, line)
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"products": items})
}

type openCartBody struct {
	StoreID string `json:"store_id"`
}

func (h *CustomerHandler) openCart(w http.ResponseWriter, r *http.Request) {
	var body openCartBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.StoreID == "" {
		httpx.WriteError(w, r, httpx.Validation("which shop?",
			map[string]string{"store_id": "required"}))
		return
	}

	cart, err := h.service.OpenCart(r.Context(),
		identity.MustPrincipal(r.Context()).UserID, body.StoreID)
	if err != nil {
		customerError(w, r, err)
		return
	}
	writeCart(w, r, cart)
}

type addItemBody struct {
	ProductID              string `json:"product_id"`
	VariantID              string `json:"variant_id,omitempty"`
	Quantity               int    `json:"quantity"`
	SubstitutionPreference string `json:"substitution_preference,omitempty"`
}

func (h *CustomerHandler) addItem(w http.ResponseWriter, r *http.Request) {
	var body addItemBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.ProductID == "" {
		httpx.WriteError(w, r, httpx.Validation("which product?",
			map[string]string{"product_id": "required"}))
		return
	}

	if _, err := h.service.AddItem(r.Context(), identity.MustPrincipal(r.Context()).UserID,
		r.PathValue("id"), body.ProductID, body.VariantID, body.Quantity,
		SubstitutionPreference(body.SubstitutionPreference)); err != nil {
		customerError(w, r, err)
		return
	}

	// The cart, not the line: a client showing a running total needs the total
	// the server computed, and asking for it separately is a round trip and a
	// chance to disagree.
	cart, err := h.service.Order(r.Context(),
		identity.MustPrincipal(r.Context()).UserID, r.PathValue("id"))
	if err != nil {
		customerError(w, r, err)
		return
	}
	writeCart(w, r, cart)
}

func (h *CustomerHandler) order(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.Order(r.Context(),
		identity.MustPrincipal(r.Context()).UserID, r.PathValue("id"))
	if err != nil {
		customerError(w, r, err)
		return
	}
	writeCart(w, r, order)
}

func (h *CustomerHandler) orders(w http.ResponseWriter, r *http.Request) {
	limit := httpx.ClampLimit(intParam(r, "limit"))
	found, err := h.service.Orders(r.Context(),
		identity.MustPrincipal(r.Context()).UserID, limit)
	if err != nil {
		customerError(w, r, err)
		return
	}

	items := make([]CartResponse, 0, len(found))
	for _, order := range found {
		item, err := toCartResponse(order)
		if err != nil {
			httpx.WriteError(w, r, httpx.Internal("could not render an order").WithCause(err))
			return
		}
		items = append(items, item)
	}
	httpx.WriteJSON(w, r, http.StatusOK,
		map[string]any{"items": items, "page": httpx.PageInfo{Limit: limit}})
}

type placeBody struct {
	Delivery struct {
		Address   string  `json:"address"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Notes     string  `json:"notes,omitempty"`
	} `json:"delivery"`
}

func (h *CustomerHandler) place(w http.ResponseWriter, r *http.Request) {
	var body placeBody
	if !decodeBody(w, r, &body) {
		return
	}

	placed, err := h.service.Place(r.Context(), identity.MustPrincipal(r.Context()).UserID,
		r.PathValue("id"), Delivery{
			Address: body.Delivery.Address,
			Lat:     body.Delivery.Latitude,
			Lon:     body.Delivery.Longitude,
			Notes:   body.Delivery.Notes,
		})
	if err != nil {
		customerError(w, r, err)
		return
	}
	writeCart(w, r, placed)
}

func writeCart(w http.ResponseWriter, r *http.Request, order Order) {
	response, err := toCartResponse(order)
	if err != nil {
		httpx.WriteError(w, r, httpx.Internal("could not render the order").WithCause(err))
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, response)
}

func decodeBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		httpx.WriteError(w, r, httpx.Validation("the request body could not be read", nil))
		return false
	}
	return true
}

func floatParam(r *http.Request, name string) (float64, error) {
	return strconv.ParseFloat(r.URL.Query().Get(name), 64)
}

// customerError maps the customer surface's sentinels onto the taxonomy.
func customerError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, httpx.NotFound("not found"))
	case errors.Is(err, ErrNotACart):
		httpx.WriteError(w, r, httpx.Conflict(
			"this order has been placed and can no longer be changed"))
	case errors.Is(err, ErrNoDestination):
		httpx.WriteError(w, r, httpx.Validation("where should this be delivered?",
			map[string]string{"delivery": "an address and its coordinates are both required"}))
	case errors.Is(err, ErrBadPoint):
		httpx.WriteError(w, r, httpx.Validation("that is not a position on Earth",
			map[string]string{"lat": "-90 to 90", "lon": "-180 to 180"}))
	case errors.Is(err, ErrBadQuantity):
		httpx.WriteError(w, r, httpx.Validation("that is not a quantity",
			map[string]string{"quantity": "1 to " + strconv.Itoa(MaxLineQuantity)}))
	case errors.Is(err, ErrBadPreference):
		httpx.WriteError(w, r, httpx.Validation("no such substitution preference",
			map[string]string{"substitution_preference": "ALLOW, DO_NOT_ALLOW or ASK_ME"}))
	case errors.Is(err, ErrOutOfStock):
		// Distinct from a missing product: the customer can pick something
		// else, and "not found" would send them looking for a typo.
		httpx.WriteError(w, r, httpx.Conflict("that is not available at this shop right now"))
	case errors.Is(err, ErrAcceptTimeoutUnset):
		httpx.WriteError(w, r, httpx.Unavailable("this shop cannot take orders right now"))
	case errors.Is(err, ErrStale):
		httpx.WriteError(w, r, httpx.Conflict("this order changed, please reload it"))
	default:
		httpx.WriteError(w, r, err)
	}
}
