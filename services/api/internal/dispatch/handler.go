package dispatch

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Handler is how a driver answers an offer.
//
// These two routes existed in booking and answered 503 — "offer responses are
// served by the dispatch surface" — and no dispatch surface was ever built. So
// once a round offered a job, the driver's app could see the offer through
// GET /driver/assignment and had no way to say yes. This is that surface, and
// it lives here because accepting is the reservation and the reservation is
// dispatch's: a second acceptance path is exactly how two drivers end up
// holding one job.
type Handler struct {
	offers   Offers
	drivers  DriverLookup
	presence Presence
	// announce tells the customer a driver said yes. Optional: the acceptance
	// is committed either way, and a customer whose stream missed it learns
	// the same thing from the next fetch.
	announce AcceptAnnouncer
	// jobs supplies the customer behind the job, so the acceptance can reach
	// their channel and not only the job's. A customer knows their own user id
	// before they know a job exists, and subscribes accordingly.
	jobs   JobLookup
	logger *slog.Logger
}

// AcceptAnnouncer publishes an accepted offer.
type AcceptAnnouncer interface {
	JobAccepted(ctx context.Context, jobID, driverID, requesterUserID, jobType string)
}

// JobLookup is the one question this handler asks of the job store.
type JobLookup interface {
	ByID(ctx context.Context, id string) (jobs.Job, error)
}

// WithAnnouncer attaches the realtime gateway an acceptance is pushed through.
func (h *Handler) WithAnnouncer(announce AcceptAnnouncer, jobStore JobLookup) *Handler {
	h.announce = announce
	h.jobs = jobStore
	return h
}

// Offers is the acceptance half of the dispatch store.
type Offers interface {
	Accept(ctx context.Context, jobID, driverID string) (jobs.Assignment, error)
	Reject(ctx context.Context, jobID, driverID string) error
}

// DriverLookup turns the authenticated user into the driver who holds offers.
type DriverLookup interface {
	DriverByUserID(ctx context.Context, userID string) (providers.Driver, error)
}

// Presence is what must happen outside the acceptance transaction.
//
// Accept's own comment says it: "the driver leaves the availability pool by
// state; the geo index is cleared by the caller, which owns Redis". The
// tracking session is here for the same reason — it is what authorises the
// customer to watch this driver, for this job, while it is live (document 102).
type Presence interface {
	RemoveFromPool(ctx context.Context, driverID string) error
	StartSession(ctx context.Context, jobID, driverID string) (tracking.Session, error)
}

func NewHandler(offers Offers, drivers DriverLookup, presence Presence,
	logger *slog.Logger) *Handler {
	return &Handler{offers: offers, drivers: drivers, presence: presence, logger: logger}
}

// Routes registers the offer responses.
//
// The paths are the ones document 035 gives and the ones the driver app
// already calls; only the code behind them is new.
func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	driverOnly := func(fn http.HandlerFunc) http.Handler {
		return authenticate(identity.RequireRole(identity.RoleDriver)(fn))
	}

	mux.Handle("POST "+p+"/driver/jobs/{id}/accept", driverOnly(h.accept))
	mux.Handle("POST "+p+"/driver/jobs/{id}/reject", driverOnly(h.reject))
}

// AcceptResponse is what the driver's app gets back.
type AcceptResponse struct {
	AssignmentID string `json:"assignment_id"`
	JobID        string `json:"job_id"`
	Status       string `json:"status"`
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	driverID, ok := h.driver(w, r)
	if !ok {
		return
	}
	jobID := r.PathValue("id")

	assignment, err := h.offers.Accept(r.Context(), jobID, driverID)
	if err != nil {
		writeOfferError(w, r, err)
		return
	}

	// Both of these follow a committed acceptance, so neither may fail the
	// request: the driver holds the job either way, and telling them
	// otherwise would have them tap again on a job they already won.
	if err := h.presence.RemoveFromPool(r.Context(), driverID); err != nil {
		h.logger.Warn("accepted driver still in the geo pool",
			slog.String("driver_id", driverID), slog.String("error", err.Error()))
	}
	if _, err := h.presence.StartSession(r.Context(), jobID, driverID); err != nil {
		// The trip works; the customer just cannot watch it yet. Louder than a
		// warning would be wrong, and silence would leave nobody to tell.
		h.logger.Error("could not open tracking for an accepted job",
			slog.String("job_id", jobID), slog.String("error", err.Error()))
	}

	h.announceAccepted(r, jobID, driverID)

	httpx.WriteJSON(w, r, http.StatusOK, AcceptResponse{
		AssignmentID: assignment.ID,
		JobID:        assignment.JobID,
		Status:       string(assignment.Status),
	})
}

// announceAccepted publishes the acceptance, and never fails the request.
//
// Like the two calls above it, this follows a committed acceptance: the driver
// holds the job whether or not anybody's screen heard about it, and document
// 047 requires that a client recover by asking rather than by trusting this
// path.
func (h *Handler) announceAccepted(r *http.Request, jobID, driverID string) {
	if h.announce == nil {
		return
	}
	requesterUserID, jobType := "", ""
	if h.jobs != nil {
		if job, err := h.jobs.ByID(r.Context(), jobID); err == nil {
			requesterUserID, jobType = job.RequesterUserID, string(job.Type)
		}
	}
	h.announce.JobAccepted(r.Context(), jobID, driverID, requesterUserID, jobType)
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	driverID, ok := h.driver(w, r)
	if !ok {
		return
	}

	if err := h.offers.Reject(r.Context(), r.PathValue("id"), driverID); err != nil {
		writeOfferError(w, r, err)
		return
	}
	// 204: the job has gone back to dispatch and there is nothing about it
	// that is still the driver's.
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) driver(w http.ResponseWriter, r *http.Request) (string, bool) {
	principal := identity.MustPrincipal(r.Context())
	driver, err := h.drivers.DriverByUserID(r.Context(), principal.UserID)
	if err != nil {
		httpx.WriteError(w, r, httpx.Forbidden("not permitted"))
		return "", false
	}
	return driver.ID, true
}

// writeOfferError maps the acceptance race onto the taxonomy.
//
// Every one of these is "you did not win", and they are deliberately not
// flattened into one message: a driver whose offer expired should refresh,
// and a driver who was suspended between the offer and the tap needs to know
// it was not a race they lost.
func writeOfferError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOfferNotFound):
		httpx.WriteError(w, r, httpx.Conflict("this offer is no longer open"))
	case errors.Is(err, ErrReservationLost), errors.Is(err, ErrJobClaimed):
		httpx.WriteError(w, r, httpx.Conflict("another driver took this job"))
	case errors.Is(err, ErrNotEligible):
		httpx.WriteError(w, r, httpx.Forbidden(
			"your account cannot take jobs right now"))
	default:
		httpx.WriteError(w, r, httpx.Internal("could not answer the offer").WithCause(err))
	}
}
