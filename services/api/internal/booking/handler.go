package booking

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Handler serves the customer and driver surfaces from documents 14 and 35.
type Handler struct {
	service   *Service
	jobs      *jobs.Store
	providers *providers.Store
}

func NewHandler(service *Service, jobStore *jobs.Store, providerStore *providers.Store) *Handler {
	return &Handler{service: service, jobs: jobStore, providers: providerStore}
}

// Routes registers the documented endpoints.
func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	auth := func(fn http.HandlerFunc) http.Handler { return authenticate(fn) }

	// Customer (document 14).
	mux.Handle("POST "+p+"/quotes", auth(h.quote))
	mux.Handle("POST "+p+"/jobs", auth(h.create))
	mux.Handle("GET "+p+"/jobs", auth(h.list))
	mux.Handle("GET "+p+"/jobs/{id}", auth(h.get))
	mux.Handle("POST "+p+"/jobs/{id}/cancel", auth(h.cancel))

	// Driver. Every command validates assignment ownership (document 35).
	driverOnly := func(fn http.HandlerFunc) http.Handler {
		return authenticate(identity.RequireRole(identity.RoleDriver)(fn))
	}
	mux.Handle("POST "+p+"/driver/jobs/{id}/accept", driverOnly(h.driverCommand(CommandAccept)))
	mux.Handle("POST "+p+"/driver/jobs/{id}/reject", driverOnly(h.driverCommand(CommandReject)))
	mux.Handle("POST "+p+"/driver/jobs/{id}/arrive", driverOnly(h.driverCommand(CommandArrive)))
	mux.Handle("POST "+p+"/driver/jobs/{id}/start", driverOnly(h.driverCommand(CommandStart)))
	mux.Handle("POST "+p+"/driver/jobs/{id}/complete", driverOnly(h.driverCommand(CommandComplete)))
}

// --- bodies ------------------------------------------------------------------

type stopBody struct {
	Type         string  `json:"type"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Address      string  `json:"address,omitempty"`
	ContactName  string  `json:"contact_name,omitempty"`
	ContactPhone string  `json:"contact_phone,omitempty"`
}

type quoteBody struct {
	JobType      string            `json:"job_type"`
	VehicleType  string            `json:"vehicle_type"`
	City         string            `json:"city,omitempty"`
	Stops        []stopBody        `json:"stops"`
	Requirements map[string]string `json:"requirements,omitempty"`
}

type createBody struct {
	QuoteID      string            `json:"quote_id"`
	JobType      string            `json:"job_type"`
	Stops        []stopBody        `json:"stops"`
	Requirements map[string]string `json:"requirements,omitempty"`
	ScheduledAt  *time.Time        `json:"scheduled_at,omitempty"`
}

type cancelBody struct {
	Reason string `json:"reason,omitempty"`
}

// JobResponse is the job shape clients receive. Exported because it is part
// of the wire contract and is generated into TypeScript (ADR-007).
type JobResponse struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Status      string         `json:"status"`
	Stops       []StopResponse `json:"stops"`
	QuoteID     string         `json:"quote_id,omitempty"`
	DriverID    string         `json:"assigned_driver_id,omitempty"`
	ScheduledAt *time.Time     `json:"scheduled_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// StopResponse is one stop as clients see it.
type StopResponse struct {
	ID        string  `json:"id"`
	Sequence  int     `json:"sequence"`
	Type      string  `json:"type"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Address   string  `json:"address,omitempty"`
}

func toJobResponse(j jobs.Job) JobResponse {
	stops := make([]StopResponse, 0, len(j.Stops))
	for _, s := range j.Stops {
		stops = append(stops, StopResponse{
			ID: s.ID, Sequence: s.Sequence, Type: string(s.Type),
			Latitude: s.Location.Latitude, Longitude: s.Location.Longitude, Address: s.Address,
		})
	}
	return JobResponse{
		ID: j.ID, Type: string(j.Type), Status: string(j.Status), Stops: stops,
		QuoteID: j.QuoteID, DriverID: j.AssignedDriverID,
		ScheduledAt: j.ScheduledAt, CreatedAt: j.CreatedAt,
	}
}

func toStops(rows []stopBody) []jobs.Stop {
	out := make([]jobs.Stop, 0, len(rows))
	for i, s := range rows {
		stopType := jobs.StopType(s.Type)
		if stopType == "" {
			// A two-stop journey is pickup then dropoff; anything longer must
			// say what each stop is.
			if i == 0 {
				stopType = jobs.StopPickup
			} else {
				stopType = jobs.StopDropoff
			}
		}
		out = append(out, jobs.Stop{
			Sequence: i, Type: stopType,
			Location:     jobs.Coordinate{Latitude: s.Latitude, Longitude: s.Longitude},
			Address:      s.Address,
			ContactName:  s.ContactName,
			ContactPhone: s.ContactPhone,
		})
	}
	return out
}

func toRequirements(rows map[string]string) []jobs.Requirement {
	out := make([]jobs.Requirement, 0, len(rows))
	for name, value := range rows {
		out = append(out, jobs.Requirement{Name: name, Value: value})
	}
	return out
}

// --- customer handlers -------------------------------------------------------

func (h *Handler) quote(w http.ResponseWriter, r *http.Request) {
	principal := identity.MustPrincipal(r.Context())
	var body quoteBody
	if !decode(w, r, &body) {
		return
	}

	quote, err := h.service.Quote(r.Context(), QuoteRequest{
		JobType: jobs.Type(body.JobType), VehicleType: body.VehicleType, City: body.City,
		Stops: toStops(body.Stops), Requirements: toRequirements(body.Requirements),
		RequestedBy: principal.UserID,
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// Written as the declared struct rather than an ad-hoc map. A map here
	// silently drifts from QuoteResponse — which it had, leaving the generated
	// client parsing fields the server never sent.
	httpx.WriteJSON(w, r, http.StatusOK, QuoteResponse{
		QuoteID:         quote.ID,
		Total:           quote.Total,
		Lines:           quote.Job.Lines,
		DistanceMeters:  quote.Job.DistanceMeters,
		DurationSeconds: quote.Job.DurationSeconds,
		// The client is told how the route was obtained, so an estimate is
		// never presented as a measured fare (document 96).
		RouteConfidence: string(quote.Job.RouteConfidence),
		ExpiresAt:       quote.Job.ExpiresAt,
	})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	principal := identity.MustPrincipal(r.Context())
	var body createBody
	if !decode(w, r, &body) {
		return
	}

	job, err := h.service.Create(r.Context(), CreateRequest{
		QuoteID: body.QuoteID, RequesterID: principal.UserID,
		JobType: jobs.Type(body.JobType), Stops: toStops(body.Stops),
		Requirements: toRequirements(body.Requirements), ScheduledAt: body.ScheduledAt,
		// Document 14 requires an Idempotency-Key on job creation.
		IdempotencyKey: r.Header.Get(httpx.IdempotencyKeyHeader),
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusCreated, toJobResponse(job))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	principal := identity.MustPrincipal(r.Context())
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

	// Always scoped to the authenticated user (document 35).
	found, err := h.jobs.ListForRequester(r.Context(), principal.UserID, before, limit)
	if err != nil {
		httpx.WriteError(w, r, httpx.Internal("could not list jobs").WithCause(err))
		return
	}

	items := make([]JobResponse, 0, len(found))
	for _, job := range found {
		items = append(items, toJobResponse(job))
	}
	page := httpx.PageInfo{Limit: limit}
	if len(found) == limit && limit > 0 {
		page.NextCursor = found[len(found)-1].CreatedAt.Format(time.RFC3339Nano)
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": items, "page": page})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	principal := identity.MustPrincipal(r.Context())
	job, err := h.jobs.ByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NotFound("job not found"))
		return
	}
	// Resource authorization: the requester, or an operator.
	if err := identity.RequireSelfOrRole(principal, job.RequesterUserID,
		identity.RoleAdmin, identity.RoleSuperAdmin, identity.RoleSupport); err != nil {
		// A driver assigned to the job may also see it.
		if !h.driverHoldsJob(r, principal, job) {
			httpx.WriteError(w, r, err)
			return
		}
	}
	httpx.WriteJSON(w, r, http.StatusOK, toJobResponse(job))
}

func (h *Handler) driverHoldsJob(r *http.Request, principal identity.Principal, job jobs.Job) bool {
	if job.AssignedDriverID == "" || !principal.HasRole(identity.RoleDriver) {
		return false
	}
	driver, err := h.providers.DriverByUserID(r.Context(), principal.UserID)
	return err == nil && driver.ID == job.AssignedDriverID
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	principal := identity.MustPrincipal(r.Context())
	var body cancelBody
	if r.ContentLength > 0 && !decode(w, r, &body) {
		return
	}

	job, cancellation, err := h.service.Cancel(r.Context(), r.PathValue("id"),
		principal.UserID, jobs.ActorCustomer, body.Reason)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, CancelResponse{
		Job:              toJobResponse(job),
		CancellationTier: string(cancellation.Tier),
		Fee:              cancellation.Fee,
	})
}

// --- driver handlers ---------------------------------------------------------

// Command values the HTTP surface adds on top of the service's lifecycle
// commands.
const (
	CommandAccept Command = "accept"
	CommandReject Command = "reject"
)

func (h *Handler) driverCommand(cmd Command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal := identity.MustPrincipal(r.Context())
		driver, err := h.providers.DriverByUserID(r.Context(), principal.UserID)
		if err != nil {
			httpx.WriteError(w, r, httpx.Forbidden("not permitted"))
			return
		}

		jobID := r.PathValue("id")
		switch cmd {
		case CommandAccept, CommandReject:
			// Accept and reject belong to dispatch, which owns the offer and
			// the reservation. The handler refuses rather than reimplementing
			// them here — a second acceptance path is exactly how two drivers
			// end up holding one job.
			httpx.WriteError(w, r, httpx.Unavailable(
				"offer responses are served by the dispatch surface"))
			return
		default:
			job, err := h.service.Execute(r.Context(), jobID, driver.ID, cmd)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			httpx.WriteJSON(w, r, http.StatusOK, toJobResponse(job))
		}
	}
}

// --- helpers -----------------------------------------------------------------

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.WriteError(w, r, httpx.Validation("the request body is not valid",
			map[string]string{"body": err.Error()}))
		return false
	}
	return true
}

func intParam(r *http.Request, name string) int {
	value := r.URL.Query().Get(name)
	if value == "" {
		return 0
	}
	var n int
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
		if n > 100000 {
			return 100000
		}
	}
	return n
}

// QuoteResponse is the priced offer clients receive.
//
// RouteConfidence is on the wire deliberately: document 096 forbids presenting
// a fallback as exact, and a client that cannot tell an estimated route from a
// measured one will show both as a firm fare.
//
// Lines carry document 034's full breakdown rather than a single figure. That
// mattered less when every fare was the sum of fixed rates; it matters now
// that BD-02's demand multiplier can move a fare, because a customer charged
// 1.3x should be able to see which line did it.
type QuoteResponse struct {
	QuoteID         string         `json:"quote_id"`
	Total           money.Amount   `json:"total"`
	Lines           []pricing.Line `json:"lines"`
	DistanceMeters  int64          `json:"distance_meters"`
	DurationSeconds int64          `json:"duration_seconds"`
	RouteConfidence string         `json:"route_confidence"`
	ExpiresAt       time.Time      `json:"expires_at"`
}

// CancelResponse reports the outcome of a cancellation.
//
// The fee is always present, including when it is zero. A customer who
// cancelled inside the free window is told they were charged nothing, rather
// than being left to infer it from a missing field.
type CancelResponse struct {
	Job              JobResponse  `json:"job"`
	CancellationTier string       `json:"cancellation_tier"`
	Fee              money.Amount `json:"fee"`
}
