package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/booking"
	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// Routes registers the driver surface.
//
// Every route is behind the driver role. These endpoints move a driver in and
// out of dispatch and report their position; a customer holding a valid token
// has no business reaching any of them.
func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	driverOnly := func(fn http.HandlerFunc) http.Handler {
		return authenticate(identity.RequireRole(identity.RoleDriver)(fn))
	}

	mux.Handle("GET "+p+"/driver/me", driverOnly(h.me))
	mux.Handle("POST "+p+"/driver/online", driverOnly(h.online))
	mux.Handle("POST "+p+"/driver/offline", driverOnly(h.offline))
	mux.Handle("POST "+p+"/driver/location", driverOnly(h.location))
	mux.Handle("GET "+p+"/driver/assignment", driverOnly(h.assignment))
	mux.Handle("GET "+p+"/driver/earnings", driverOnly(h.earnings))
	// What the driver is holding for the platform, and whether it stops them
	// working (BD-09). Its own route rather than a field on /driver/me: a
	// driver checks this when they are about to be stopped, and it must not
	// be a side effect of a call they make for another reason.
	mux.Handle("GET "+p+"/driver/balance", driverOnly(h.balance))
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	d, err := h.service.Me(r.Context(), identity.MustPrincipal(r.Context()).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, toDriverResponse(d))
}

type onlineBody struct {
	Latitude   float64  `json:"latitude"`
	Longitude  float64  `json:"longitude"`
	AccuracyM  *float64 `json:"accuracy_m,omitempty"`
	HeadingDeg *float64 `json:"heading_deg,omitempty"`
	SpeedMPS   *float64 `json:"speed_mps,omitempty"`
	RecordedAt *string  `json:"recorded_at,omitempty"`
}

func (h *Handler) online(w http.ResponseWriter, r *http.Request) {
	var body onlineBody
	if !decode(w, r, &body) {
		return
	}
	fix := tracking.Fix{
		Lat: body.Latitude, Lon: body.Longitude,
		AccuracyM: body.AccuracyM, HeadingDeg: body.HeadingDeg, SpeedMPS: body.SpeedMPS,
	}
	if body.RecordedAt != nil {
		at, err := time.Parse(time.RFC3339, *body.RecordedAt)
		if err != nil {
			httpx.WriteError(w, r, httpx.Validation("recorded_at is not a valid timestamp",
				map[string]string{"recorded_at": "expected RFC3339"}))
			return
		}
		fix.RecordedAt = at
	}

	d, err := h.service.GoOnline(r.Context(), identity.MustPrincipal(r.Context()).UserID, fix)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, toDriverResponse(d))
}

func (h *Handler) offline(w http.ResponseWriter, r *http.Request) {
	d, err := h.service.GoOffline(r.Context(), identity.MustPrincipal(r.Context()).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, toDriverResponse(d))
}

type locationBody struct {
	Fixes []struct {
		Latitude   float64  `json:"latitude"`
		Longitude  float64  `json:"longitude"`
		AccuracyM  *float64 `json:"accuracy_m,omitempty"`
		HeadingDeg *float64 `json:"heading_deg,omitempty"`
		SpeedMPS   *float64 `json:"speed_mps,omitempty"`
		JobID      string   `json:"job_id,omitempty"`
		RecordedAt string   `json:"recorded_at"`
	} `json:"fixes"`
}

func (h *Handler) location(w http.ResponseWriter, r *http.Request) {
	var body locationBody
	if !decode(w, r, &body) {
		return
	}
	if len(body.Fixes) == 0 {
		httpx.WriteError(w, r, httpx.Validation("no fixes were sent",
			map[string]string{"fixes": "at least one required"}))
		return
	}

	fixes := make([]tracking.Fix, 0, len(body.Fixes))
	for i, raw := range body.Fixes {
		at, err := time.Parse(time.RFC3339, raw.RecordedAt)
		if err != nil {
			httpx.WriteError(w, r, httpx.Validation("a fix has an invalid timestamp",
				map[string]string{"fixes": "expected RFC3339 at index " + strconv.Itoa(i)}))
			return
		}
		fixes = append(fixes, tracking.Fix{
			Lat: raw.Latitude, Lon: raw.Longitude, JobID: raw.JobID,
			AccuracyM: raw.AccuracyM, HeadingDeg: raw.HeadingDeg, SpeedMPS: raw.SpeedMPS,
			RecordedAt: at,
		})
	}

	accepted, rejected, err := h.service.ReportLocation(
		r.Context(), identity.MustPrincipal(r.Context()).UserID, fixes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if rejected == nil {
		rejected = []RejectedFix{}
	}
	httpx.WriteJSON(w, r, http.StatusOK, LocationResponse{Accepted: accepted, Rejected: rejected})
}

func (h *Handler) assignment(w http.ResponseWriter, r *http.Request) {
	current, err := h.service.Current(r.Context(), identity.MustPrincipal(r.Context()).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, AssignmentResponse{
		ID:        current.Assignment.ID,
		Status:    string(current.Assignment.Status),
		OfferedAt: current.Assignment.OfferedAt,
		ExpiresAt: current.Assignment.ExpiresAt,
		Job:       booking.ToJobResponse(current.Job),
	})
}

// --- responses ---------------------------------------------------------------

// DriverResponse is a driver's own record.
//
// It carries the operational status and nothing about verification documents
// or ratings: this is what the driver app needs to decide which screen to
// show, and every extra field is one more thing to keep in sync.
type DriverResponse struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	ActiveVehicleID string `json:"active_vehicle_id,omitempty"`
	Verification    string `json:"verification_status"`
}

func toDriverResponse(d providers.Driver) DriverResponse {
	return DriverResponse{
		ID: d.ID, Status: string(d.Status),
		ActiveVehicleID: d.ActiveVehicleID,
		Verification:    string(d.VerificationStatus),
	}
}

// LocationResponse reports what a batch of fixes did.
//
// Rejections are returned rather than swallowed, so a driver app can tell "we
// dropped your GPS spike" from "we lost your report" — and so a phone that is
// consistently rejected can say something rather than reporting into nothing.
type LocationResponse struct {
	Accepted int           `json:"accepted"`
	Rejected []RejectedFix `json:"rejected"`
}

// AssignmentResponse is the offer or trip a driver is holding.
//
// ExpiresAt is what the offer countdown is drawn from. Without it the app
// would have to guess the TTL, and a countdown that disagrees with the server
// is worse than none.
type AssignmentResponse struct {
	ID        string              `json:"id"`
	Status    string              `json:"status"`
	OfferedAt time.Time           `json:"offered_at"`
	ExpiresAt *time.Time          `json:"expires_at,omitempty"`
	Job       booking.JobResponse `json:"job"`
}

// --- helpers -----------------------------------------------------------------

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		httpx.WriteError(w, r, httpx.Validation("the request body could not be read", nil))
		return false
	}
	return true
}

// writeError maps this package's sentinels onto the HTTP taxonomy.
//
// Anything unrecognised falls through to httpx, which already distinguishes
// its own typed errors from genuine internals.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotADriver):
		httpx.WriteError(w, r, httpx.Forbidden("this account is not a driver"))
	case errors.Is(err, ErrNoVehicle):
		httpx.WriteError(w, r, httpx.Conflict(
			"select an active vehicle before going online"))
	case errors.Is(err, ErrNoLedger):
		httpx.WriteError(w, r, httpx.Unavailable("balances are not available right now"))
	case errors.Is(err, providers.ErrStale):
		httpx.WriteError(w, r, httpx.Conflict("your status changed, please retry"))
	default:
		// BD-09's refusal. 409 rather than 403: nothing is wrong with who the
		// driver is, and the state that refuses them is one they can change.
		// The figures travel in the details so the app can say the amount
		// without a second request.
		var overCap *ErrOverCreditCap
		if errors.As(err, &overCap) {
			httpx.WriteError(w, r, httpx.Conflict(
				"hand in platform cash before going online").WithDetails(map[string]string{
				"owed":     overCap.Owed.String(),
				"cap":      overCap.Cap.String(),
				"clearing": overCap.Clearing.String(),
			}))
			return
		}
		var rejected *tracking.ErrRejected
		if errors.As(err, &rejected) {
			httpx.WriteError(w, r, httpx.Validation("the position was rejected",
				map[string]string{"location": string(rejected.Reason)}))
			return
		}
		httpx.WriteError(w, r, err)
	}
}

// earnings answers "what have I made", read from the ledger.
//
// A driver asks this at the end of a shift and after a trip they think was
// underpaid. The second question is why the individual trips travel with the
// totals: a figure with no breakdown cannot answer it.
// BalanceResponse is what the driver owes and what it means for them.
type BalanceResponse struct {
	Standing finance.Standing `json:"standing"`
	// Message is the sentence the app shows. Composed here so the wording is
	// one thing in one place, rather than three clients each inventing their
	// own way to say "you cannot work".
	Message string `json:"message"`
}

func (h *Handler) balance(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}
	standing, err := h.service.Standing(r.Context(), principal.UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, BalanceResponse{
		Standing: standing,
		Message:  balanceMessage(standing),
	})
}

// balanceMessage says what is true, plainly, and always says what to do next.
func balanceMessage(s finance.Standing) string {
	switch {
	case !s.Limited:
		return fmt.Sprintf("You are holding %s in platform cash.", s.Owed)
	case s.Blocked:
		return fmt.Sprintf(
			"You are holding %s, which is at your limit of %s. Hand in %s to start accepting trips again.",
			s.Owed, s.Cap, s.Clearing)
	case s.Warning:
		return fmt.Sprintf(
			"You are holding %s of your %s limit. Hand in cash soon to avoid being paused.",
			s.Owed, s.Cap)
	default:
		return fmt.Sprintf("You are holding %s of your %s limit.", s.Owed, s.Cap)
	}
}

func (h *Handler) earnings(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.EarningsFor(r.Context(), identity.MustPrincipal(r.Context()).UserID)
	if err != nil {
		if errors.Is(err, ErrNoLedger) {
			// Distinct from "you earned nothing": a driver told zero when the
			// books were simply unreachable would believe they worked for
			// free.
			httpx.WriteError(w, r, httpx.Unavailable("earnings are unavailable right now"))
			return
		}
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, result)
}
