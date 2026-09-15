// Package credit is the operator's side of BD-09.
//
// The cap built on 2026-09-15 could stop a driver working and nothing could
// start them again: the ledger knew how to record a repayment and no route
// called it. A driver over their limit was stuck until somebody ran SQL.
//
// This is the counter an agent stands behind. It is deliberately a package of
// its own rather than a handler inside `finance`: finance holds no HTTP, and
// the thing that decides who may take money from a driver is an authorization
// question rather than an accounting one.
package credit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Ledger is what this surface needs of the books.
type Ledger interface {
	StandingOf(ctx context.Context, driverID, vehicleType string) (finance.Standing, error)
	RecordSettlement(ctx context.Context, set finance.Settlement, idempotencyKey string) (finance.Settlement, error)
	SettlementsOf(ctx context.Context, driverID string, limit int) ([]finance.Settlement, error)
	SetCreditLimit(ctx context.Context, limit finance.CreditLimit) error
	LimitFor(ctx context.Context, vehicleType, driverID string) (finance.CreditLimit, error)
}

// Drivers resolves the vehicle a driver works on, because the cap that applies
// to them is the cap for that vehicle type.
type Drivers interface {
	DriverByID(ctx context.Context, id string) (providers.Driver, error)
	DriverByUserID(ctx context.Context, userID string) (providers.Driver, error)
	VehicleByID(ctx context.Context, id string) (providers.Vehicle, error)
}

type Handler struct {
	ledger  Ledger
	drivers Drivers
}

func NewHandler(ledger Ledger, drivers Drivers) *Handler {
	return &Handler{ledger: ledger, drivers: drivers}
}

// MaxNote bounds what an agent types about a handover.
const MaxNote = 500

// Routes registers the counter.
//
// The role split is the point of this file. Support can *see* what a driver
// owes, because "why can I not go online" is the call they take. Support
// cannot record that money arrived and cannot change a cap: both are
// money-moving acts, and an agent who can mark a debt settled without cash
// changing hands is a fraud path with a help-desk login.
func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix

	canSee := func(fn http.HandlerFunc) http.Handler {
		return authenticate(identity.RequireRole(
			identity.RoleAdmin, identity.RoleSuperAdmin, identity.RoleSupport)(fn))
	}
	canTakeMoney := func(fn http.HandlerFunc) http.Handler {
		return authenticate(identity.RequireRole(identity.RoleAdmin, identity.RoleSuperAdmin)(fn))
	}
	driverOnly := func(fn http.HandlerFunc) http.Handler {
		return authenticate(identity.RequireRole(identity.RoleDriver)(fn))
	}

	mux.Handle("GET "+p+"/admin/drivers/{id}/balance", canSee(h.balance))
	mux.Handle("GET "+p+"/admin/drivers/{id}/settlements", canSee(h.history))
	mux.Handle("POST "+p+"/admin/drivers/{id}/settlements", canTakeMoney(h.record))
	mux.Handle("GET "+p+"/admin/credit-limits", canSee(h.limits))
	mux.Handle("PUT "+p+"/admin/credit-limits", canTakeMoney(h.setLimit))

	// A driver's own record of what they have handed in. They are the party
	// most likely to dispute it, and the least able to ask a database.
	mux.Handle("GET "+p+"/driver/settlements", driverOnly(h.myHistory))
}

func (h *Handler) balance(w http.ResponseWriter, r *http.Request) {
	driverID := r.PathValue("id")
	standing, err := h.standingOf(r.Context(), driverID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{
		"driver_id": driverID,
		"standing":  standing,
	})
}

// RecordSettlementRequest is an agent saying money arrived.
type RecordSettlementRequest struct {
	AmountMinor int64  `json:"amount_minor"`
	Method      string `json:"method"`
	Reference   string `json:"reference,omitempty"`
	Note        string `json:"note,omitempty"`
}

// RecordSettlementResponse returns the driver's position afterwards, so the
// agent can tell them they are back on the road without a second call.
type RecordSettlementResponse struct {
	Settlement finance.Settlement `json:"settlement"`
	Standing   finance.Standing   `json:"standing"`
}

func (h *Handler) record(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}
	var body RecordSettlementRequest
	if !decode(w, r, &body) {
		return
	}
	if body.AmountMinor <= 0 {
		httpx.WriteError(w, r, httpx.Validation("a settlement needs an amount",
			map[string]string{"amount_minor": "must be greater than zero"}))
		return
	}
	if len(body.Note) > MaxNote {
		httpx.WriteError(w, r, httpx.Validation("the note is too long",
			map[string]string{"note": "at most 500 characters"}))
		return
	}
	method := body.Method
	if method == "" {
		// Cash is what actually happens at a counter. Defaulting to it beats
		// refusing an agent who is holding notes and did not fill a field in.
		method = "CASH"
	}

	amount, err := money.New(body.AmountMinor, money.PKR)
	if err != nil {
		httpx.WriteError(w, r, httpx.Validation("the amount is not valid",
			map[string]string{"amount_minor": err.Error()}))
		return
	}

	driverID := r.PathValue("id")
	// The driver must exist before money is recorded against them. A typo in a
	// uuid would otherwise create a settlement nobody can find and a ledger
	// entry against a subject that is not there.
	if _, err := h.drivers.DriverByID(r.Context(), driverID); err != nil {
		writeError(w, r, err)
		return
	}

	// Document 035's header, for the reason it exists: an agent whose tap did
	// not appear to work must not be able to credit a driver twice by trying
	// again.
	key := r.Header.Get("Idempotency-Key")

	settled, err := h.ledger.RecordSettlement(r.Context(), finance.Settlement{
		DriverID: driverID, Amount: amount, Method: method,
		Reference: body.Reference, RecordedBy: principal.UserID, Note: body.Note,
	}, key)
	if err != nil {
		writeError(w, r, err)
		return
	}

	standing, err := h.standingOf(r.Context(), driverID)
	if err != nil {
		// The money is recorded; only the follow-up read failed. Reporting an
		// error here would have the agent take the cash again.
		httpx.WriteJSON(w, r, http.StatusCreated, RecordSettlementResponse{Settlement: settled})
		return
	}
	httpx.WriteJSON(w, r, http.StatusCreated, RecordSettlementResponse{
		Settlement: settled, Standing: standing,
	})
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	h.writeHistory(w, r, r.PathValue("id"))
}

func (h *Handler) myHistory(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}
	driver, err := h.drivers.DriverByUserID(r.Context(), principal.UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.writeHistory(w, r, driver.ID)
}

func (h *Handler) writeHistory(w http.ResponseWriter, r *http.Request, driverID string) {
	settlements, err := h.ledger.SettlementsOf(r.Context(), driverID, 50)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"settlements": settlements})
}

// SetLimitRequest sets the cap for one vehicle type.
type SetLimitRequest struct {
	VehicleType string `json:"vehicle_type"`
	CapMinor    int64  `json:"cap_minor"`
	WarnMinor   int64  `json:"warn_minor"`
	Version     int    `json:"version,omitempty"`
}

func (h *Handler) setLimit(w http.ResponseWriter, r *http.Request) {
	var body SetLimitRequest
	if !decode(w, r, &body) {
		return
	}
	if body.VehicleType == "" {
		httpx.WriteError(w, r, httpx.Validation("a limit needs a vehicle type",
			map[string]string{"vehicle_type": "required"}))
		return
	}
	if body.CapMinor <= 0 || body.WarnMinor <= 0 || body.WarnMinor > body.CapMinor {
		httpx.WriteError(w, r, httpx.Validation(
			"a cap must be positive and the warning must sit at or below it",
			map[string]string{"cap_minor": "positive", "warn_minor": "positive, not above the cap"}))
		return
	}

	capAmount, err := money.New(body.CapMinor, money.PKR)
	if err != nil {
		httpx.WriteError(w, r, httpx.Validation("the cap is not valid",
			map[string]string{"cap_minor": err.Error()}))
		return
	}
	warn, err := money.New(body.WarnMinor, money.PKR)
	if err != nil {
		httpx.WriteError(w, r, httpx.Validation("the warning level is not valid",
			map[string]string{"warn_minor": err.Error()}))
		return
	}

	if err := h.ledger.SetCreditLimit(r.Context(), finance.CreditLimit{
		VehicleType: body.VehicleType, Cap: capAmount, Warn: warn, Version: body.Version,
	}); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// VehicleTypes is the list a cap may be set for, mirroring the migration's
// CHECK constraint. Returned with the limits so an operator console can render
// the types that have no cap yet — which is the state that matters, because a
// type with no row has no limit at all.
var VehicleTypes = []string{
	"MOTORCYCLE", "RICKSHAW", "CAR", "PICKUP", "MAZDA", "SHEHZORE", "TRUCK",
}

func (h *Handler) limits(w http.ResponseWriter, r *http.Request) {
	configured := make([]finance.CreditLimit, 0, len(VehicleTypes))
	uncapped := make([]string, 0, len(VehicleTypes))

	for _, vehicleType := range VehicleTypes {
		limit, err := h.ledger.LimitFor(r.Context(), vehicleType, "")
		switch {
		case errors.Is(err, finance.ErrNoCap):
			uncapped = append(uncapped, vehicleType)
		case err != nil:
			writeError(w, r, err)
			return
		default:
			configured = append(configured, limit)
		}
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{
		"limits": configured,
		// Named explicitly rather than left to be inferred from absence: a
		// vehicle type with no cap has no limit at all, which is the fact an
		// operator most needs to see on this screen.
		"uncapped": uncapped,
	})
}

func (h *Handler) standingOf(ctx context.Context, driverID string) (finance.Standing, error) {
	driver, err := h.drivers.DriverByID(ctx, driverID)
	if err != nil {
		return finance.Standing{}, err
	}
	vehicleType := ""
	if driver.ActiveVehicleID != "" {
		if vehicle, verr := h.drivers.VehicleByID(ctx, driver.ActiveVehicleID); verr == nil {
			vehicleType = vehicle.Type
		}
	}
	return h.ledger.StandingOf(ctx, driverID, vehicleType)
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		httpx.WriteError(w, r, httpx.Validation("could not read the request body",
			map[string]string{"body": err.Error()}))
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, providers.ErrDriverNotFound):
		httpx.WriteError(w, r, httpx.NotFound("driver not found"))
	case errors.Is(err, finance.ErrNotFound):
		httpx.WriteError(w, r, httpx.NotFound("not found"))
	default:
		httpx.WriteError(w, r, httpx.Internal("could not complete the request").WithCause(err))
	}
}
