package tracking

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Handler lets a customer watch the driver coming for them.
//
// Everything under it existed and was unreachable: AuthorizeView, the audit
// log it writes, LiveSession, and Current. Document 018 makes live tracking
// part of the ride, and until now the only position endpoint on the platform
// was the one a driver posts into.
type Handler struct {
	positions Positions
	drivers   DriverLookup
}

// Positions is what the tracking surface needs from the store.
type Positions interface {
	LiveSession(ctx context.Context, jobID string) (Session, bool, error)
	AuthorizeView(ctx context.Context, actorID, actorRole, driverID, jobID string, scope Scope) error
	Current(ctx context.Context, driverID string) (Current, bool, error)
}

// DriverLookup tells a caller apart from the driver they are watching.
type DriverLookup interface {
	DriverIDForUser(ctx context.Context, userID string) (string, error)
}

func NewHandler(positions Positions, drivers DriverLookup) *Handler {
	return &Handler{positions: positions, drivers: drivers}
}

func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	mux.Handle("GET "+p+"/jobs/{id}/track", authenticate(http.HandlerFunc(h.track)))
}

// StaleAfter is when a position stops being worth showing as live.
//
// A marker that has not moved for two minutes is not a driver standing still,
// it is a phone that lost signal, and a customer watching a frozen marker
// believes the first thing rather than the second. The position is still
// returned — the last known place is useful — but it is labelled.
const StaleAfter = 2 * time.Minute

// TrackResponse is where the driver is.
type TrackResponse struct {
	JobID      string    `json:"job_id"`
	DriverID   string    `json:"driver_id"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	HeadingDeg *float64  `json:"heading_deg,omitempty"`
	SpeedMPS   *float64  `json:"speed_mps,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
	// Stale says the position is the last one known rather than a current one,
	// so a client can say "last seen 4 minutes ago" instead of drawing a car
	// that is not there.
	Stale bool `json:"stale"`
}

func (h *Handler) track(w http.ResponseWriter, r *http.Request) {
	principal := identity.MustPrincipal(r.Context())
	jobID := r.PathValue("id")

	// The session is what makes a job trackable, and its absence is the
	// ordinary state of every job that has not been accepted yet or has
	// already finished.
	session, live, err := h.positions.LiveSession(r.Context(), jobID)
	if err != nil {
		httpx.WriteError(w, r, httpx.Internal("could not load the trip").WithCause(err))
		return
	}
	if !live {
		httpx.WriteError(w, r, httpx.NotFound("this trip is not being tracked"))
		return
	}

	scope, role := h.scopeFor(r.Context(), principal, session.DriverID)
	if err := h.positions.AuthorizeView(r.Context(), principal.UserID,
		role, session.DriverID, jobID, scope); err != nil {
		if errors.Is(err, ErrNotPermitted) {
			// Not a 404: the caller asked about a job by id and the honest
			// answer is that watching it is not theirs to do. Document 102
			// has already recorded the refusal by the time this is written.
			httpx.WriteError(w, r, httpx.Forbidden("not permitted"))
			return
		}
		httpx.WriteError(w, r, httpx.Internal("could not check permission").WithCause(err))
		return
	}

	current, found, err := h.positions.Current(r.Context(), session.DriverID)
	if err != nil {
		httpx.WriteError(w, r, httpx.Internal("could not load the position").WithCause(err))
		return
	}
	if !found {
		// The trip is live and the driver's phone has told us nothing yet —
		// starting a shift indoors, or a dead spot. Distinct from an untracked
		// job, and a client should keep asking rather than give up.
		httpx.WriteError(w, r, httpx.Unavailable("the driver's position is not known yet"))
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, TrackResponse{
		JobID:      jobID,
		DriverID:   session.DriverID,
		Latitude:   current.Lat,
		Longitude:  current.Lon,
		HeadingDeg: current.HeadingDeg,
		SpeedMPS:   current.SpeedMPS,
		RecordedAt: current.RecordedAt,
		Stale:      !current.Fresh(time.Now(), StaleAfter),
	})
}

// scopeFor picks the document 102 scope the caller is asking under.
//
// The scope decides which authorization query runs, so getting it wrong is
// either a leak or a locked-out customer. Operations first because an admin is
// also somebody's customer; the driver next, because a driver watching their
// own job is not watching a stranger; the customer last, and that query checks
// the job is theirs and still live.
// It returns the role alongside the scope, and that role is the one that
// justified the choice rather than whichever the token happened to list first.
// An operator who is also a customer holds both, and passing CUSTOMER with
// ScopeOperations would refuse the operator their own console.
func (h *Handler) scopeFor(ctx context.Context, principal identity.Principal,
	driverID string) (Scope, string) {
	for _, role := range []identity.Role{
		identity.RoleAdmin, identity.RoleSuperAdmin, identity.RoleSupport,
	} {
		if principal.HasRole(role) {
			return ScopeOperations, string(role)
		}
	}
	if principal.HasRole(identity.RoleDriver) {
		if own, err := h.drivers.DriverIDForUser(ctx, principal.UserID); err == nil && own == driverID {
			return ScopeAssignedJob, string(identity.RoleDriver)
		}
	}
	return ScopeOwnJob, string(identity.RoleCustomer)
}
