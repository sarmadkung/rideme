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

// Handler serves document 072's merchant order management.
//
// Every route is behind the MERCHANT role and then scoped to the merchant the
// caller actually operates. The role says "a merchant is calling"; it does not
// say which one, and an order belonging to another shop must be as invisible
// as one that does not exist.
type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	merchantOnly := func(fn http.HandlerFunc) http.Handler {
		return authenticate(identity.RequireRole(identity.RoleMerchant)(fn))
	}

	mux.Handle("GET "+p+"/merchant/orders", merchantOnly(h.queue))
	mux.Handle("GET "+p+"/merchant/orders/{id}", merchantOnly(h.order))
	mux.Handle("POST "+p+"/merchant/orders/{id}/accept", merchantOnly(h.accept))
	mux.Handle("POST "+p+"/merchant/orders/{id}/reject", merchantOnly(h.reject))
	mux.Handle("POST "+p+"/merchant/orders/{id}/preparing", merchantOnly(h.preparing))
	mux.Handle("POST "+p+"/merchant/orders/{id}/ready", merchantOnly(h.ready))
}

// --- responses ---------------------------------------------------------------

// OrderItemResponse is one line of an order.
type OrderItemResponse struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Quantity  int          `json:"quantity"`
	UnitPrice money.Amount `json:"unit_price"`
	LineTotal money.Amount `json:"line_total"`
	// SubstitutionPreference is the customer's standing instruction for this
	// line (document 074). A picker needs it before they hit a gap on the
	// shelf, not after.
	SubstitutionPreference string `json:"substitution_preference"`
	Status                 string `json:"status"`
}

// OrderResponse is an order as its merchant sees it.
//
// It carries no customer identity. A shop needs to know what to pick, by when,
// and where it is going once delivery is modelled — not who ordered it.
type OrderResponse struct {
	ID         string       `json:"id"`
	Status     string       `json:"status"`
	ItemsTotal money.Amount `json:"items_total"`
	// AcceptDeadline is when an unanswered order is cancelled for the merchant
	// (BD-12). It is the single most useful field in the New queue.
	AcceptDeadline       *time.Time          `json:"accept_deadline,omitempty"`
	AcceptedAt           *time.Time          `json:"accepted_at,omitempty"`
	PreparationStartedAt *time.Time          `json:"preparation_started_at,omitempty"`
	ExpectedReadyAt      *time.Time          `json:"expected_ready_at,omitempty"`
	RejectionReason      string              `json:"rejection_reason,omitempty"`
	CreatedAt            time.Time           `json:"created_at"`
	Items                []OrderItemResponse `json:"items,omitempty"`
	// JobID is the delivery this order produced, once it is ready. Present so
	// a dashboard can follow the driver without asking a second endpoint which
	// job to follow.
	JobID string `json:"job_id,omitempty"`
}

func toOrderResponse(order Order, withItems bool) (OrderResponse, error) {
	out := OrderResponse{
		ID:                   order.ID,
		JobID:                order.JobID,
		Status:               string(order.Status),
		ItemsTotal:           order.ItemsTotal,
		AcceptDeadline:       order.AcceptDeadline,
		AcceptedAt:           order.AcceptedAt,
		PreparationStartedAt: order.PreparationStart,
		ExpectedReadyAt:      order.ExpectedReadyAt,
		RejectionReason:      order.RejectionReason,
		CreatedAt:            order.CreatedAt,
	}
	if !withItems {
		return out, nil
	}
	out.Items = make([]OrderItemResponse, 0, len(order.Items))
	for _, item := range order.Items {
		line, err := item.LineTotal()
		if err != nil {
			return OrderResponse{}, err
		}
		out.Items = append(out.Items, OrderItemResponse{
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

// DefaultQueue is what a dashboard opens on: the orders waiting on an answer.
const DefaultQueue = QueueNew

func (h *Handler) queue(w http.ResponseWriter, r *http.Request) {
	queue := DefaultQueue
	if raw := r.URL.Query().Get("queue"); raw != "" {
		queue = Queue(raw)
	}
	limit := httpx.ClampLimit(intParam(r, "limit"))

	var before *time.Time
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		parsed, err := time.Parse(time.RFC3339Nano, cursor)
		if err != nil {
			httpx.WriteError(w, r, httpx.Validation("the cursor is not valid",
				map[string]string{"cursor": "expected an RFC 3339 timestamp"}))
			return
		}
		before = &parsed
	}

	orders, err := h.service.Queue(r.Context(), identity.MustPrincipal(r.Context()).UserID,
		queue, before, limit)
	if err != nil {
		writeError(w, r, err)
		return
	}

	items := make([]OrderResponse, 0, len(orders))
	for _, order := range orders {
		// Without lines: see Store.OrdersFor. A queue is counts and deadlines.
		item, err := toOrderResponse(order, false)
		if err != nil {
			httpx.WriteError(w, r, httpx.Internal("could not render an order").WithCause(err))
			return
		}
		items = append(items, item)
	}
	page := httpx.PageInfo{Limit: limit}
	if len(orders) == limit && limit > 0 {
		page.NextCursor = orders[len(orders)-1].CreatedAt.Format(time.RFC3339Nano)
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": items, "page": page})
}

func (h *Handler) order(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.Order(r.Context(),
		identity.MustPrincipal(r.Context()).UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOrder(w, r, order)
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.Accept(r.Context(),
		identity.MustPrincipal(r.Context()).UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOrder(w, r, order)
}

type rejectBody struct {
	Reason string `json:"reason"`
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	var body rejectBody
	// An absent body is a missing reason, which the service refuses with the
	// same message as an empty one — the merchant needs to say why either way.
	_ = json.NewDecoder(r.Body).Decode(&body)

	order, err := h.service.Reject(r.Context(),
		identity.MustPrincipal(r.Context()).UserID, r.PathValue("id"), body.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOrder(w, r, order)
}

func (h *Handler) preparing(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.StartPreparing(r.Context(),
		identity.MustPrincipal(r.Context()).UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOrder(w, r, order)
}

// ready hands a finished order to a driver.
func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	order, _, err := h.service.MarkReady(r.Context(),
		identity.MustPrincipal(r.Context()).UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOrder(w, r, order)
}

func writeOrder(w http.ResponseWriter, r *http.Request, order Order) {
	response, err := toOrderResponse(order, true)
	if err != nil {
		httpx.WriteError(w, r, httpx.Internal("could not render the order").WithCause(err))
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, response)
}

func intParam(r *http.Request, name string) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return 0
	}
	return value
}

// writeError maps this package's sentinels onto the HTTP taxonomy.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, httpx.NotFound("order not found"))
	case errors.Is(err, ErrNotAMerchant):
		httpx.WriteError(w, r, httpx.Forbidden("this account does not operate a merchant yet"))
	case errors.Is(err, ErrManyMerchants):
		httpx.WriteError(w, r, httpx.Conflict(
			"this account operates more than one merchant, which this surface cannot yet select between"))
	case errors.Is(err, ErrNotActive):
		httpx.WriteError(w, r, httpx.Forbidden("this merchant is not active"))
	case errors.Is(err, ErrNoSuchQueue):
		httpx.WriteError(w, r, httpx.Validation("no such queue",
			map[string]string{"queue": "expected new, preparing, ready, completed or cancelled"}))
	case errors.Is(err, ErrReasonRequired):
		httpx.WriteError(w, r, httpx.Validation("a rejection needs a reason",
			map[string]string{"reason": "required"}))
	case errors.Is(err, ErrAwaitingPayment):
		httpx.WriteError(w, r, httpx.Conflict("this order is waiting on payment"))
	case errors.Is(err, ErrNotAcceptable):
		httpx.WriteError(w, r, httpx.Conflict("this order is not waiting to be accepted"))
	case errors.Is(err, ErrNotPreparable):
		httpx.WriteError(w, r, httpx.Conflict("accept this order before preparing it"))
	case errors.Is(err, ErrNotReadyable):
		httpx.WriteError(w, r, httpx.Conflict("start preparing this order before marking it ready"))
	case errors.Is(err, ErrNoDestination):
		// An order placed before checkout captured an address. The merchant
		// cannot fix it, so it says who can.
		httpx.WriteError(w, r, httpx.Conflict(
			"this order has no delivery address — support has to add one before it can go out"))
	case errors.Is(err, ErrNoPickupPoint):
		httpx.WriteError(w, r, httpx.Conflict(
			"this store has no location on the map, so no driver can be sent to it"))
	case errors.Is(err, ErrJobAlreadyAttached):
		httpx.WriteError(w, r, httpx.Conflict("this order already has a delivery"))
	case errors.Is(err, ErrNoDeliveries):
		httpx.WriteError(w, r, httpx.Unavailable("deliveries are unavailable right now"))
	case errors.Is(err, ErrNotCancellable):
		httpx.WriteError(w, r, httpx.Conflict(
			"this order can no longer be rejected — it is already being prepared"))
	case errors.Is(err, ErrStale):
		httpx.WriteError(w, r, httpx.Conflict("this order changed, please reload it"))
	default:
		httpx.WriteError(w, r, err)
	}
}
