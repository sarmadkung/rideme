package notify

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Handler is how a phone tells the platform where to reach it.
type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	mux.Handle("POST "+p+"/me/devices", authenticate(http.HandlerFunc(h.registerDevice)))
	mux.Handle("GET "+p+"/me/devices", authenticate(http.HandlerFunc(h.listDevices)))
	mux.Handle("DELETE "+p+"/me/devices/{deviceId}", authenticate(http.HandlerFunc(h.revokeDevice)))
	mux.Handle("GET "+p+"/me/notification-preferences", authenticate(http.HandlerFunc(h.preferences)))
	mux.Handle("PUT "+p+"/me/notification-preferences", authenticate(http.HandlerFunc(h.setPreference)))
	mux.Handle("GET "+p+"/me/notifications", authenticate(http.HandlerFunc(h.inbox)))
}

// RegisterDeviceRequest is document 122's device record, minus the fields the
// server owns.
type RegisterDeviceRequest struct {
	DeviceID   string   `json:"device_id"`
	Platform   Platform `json:"platform"`
	PushToken  string   `json:"push_token"`
	AppVersion string   `json:"app_version,omitempty"`
}

func (h *Handler) registerDevice(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}
	var body RegisterDeviceRequest
	if !decode(w, r, &body) {
		return
	}

	device, err := h.store.RegisterDevice(r.Context(), Device{
		UserID: principal.UserID, DeviceID: body.DeviceID, Platform: body.Platform,
		PushToken: body.PushToken, AppVersion: body.AppVersion,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, device)
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}
	devices, err := h.store.DevicesOf(r.Context(), principal.UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"devices": devices})
}

func (h *Handler) revokeDevice(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}
	// Scoped to the caller's own devices in the query itself, so a device id
	// belonging to somebody else is a not-found rather than a revocation.
	if err := h.store.RevokeDevice(r.Context(), principal.UserID, r.PathValue("deviceId")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) preferences(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}
	stored, err := h.store.PreferencesOf(r.Context(), principal.UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// The required list travels with the settings so a client can render the
	// switches it must not offer, rather than discovering them by being
	// refused.
	required := make([]Category, 0, 5)
	for _, c := range []Category{CategoryRide, CategoryDelivery, CategoryOrder,
		CategoryPayment, CategorySafety, CategorySupport, CategoryMarketing} {
		if c.Required() {
			required = append(required, c)
		}
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{
		"preferences": stored,
		"required":    required,
	})
}

func (h *Handler) setPreference(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}
	var body Preference
	if !decode(w, r, &body) {
		return
	}
	if err := h.store.SetPreference(r.Context(), principal.UserID,
		body.Channel, body.Category, body.Enabled); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) inbox(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
		return
	}
	items, err := h.store.Recent(r.Context(), principal.UserID, 50)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, n := range items {
		out = append(out, map[string]any{
			"id": n.ID, "channel": n.Channel, "category": n.Category,
			"title": n.Title, "body": n.Body, "deep_link": n.DeepLink,
			"status": n.Status, "created_at": n.CreatedAt,
		})
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"notifications": out})
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
	case errors.Is(err, ErrInvalid):
		httpx.WriteError(w, r, httpx.Validation(err.Error(), nil))
	case errors.Is(err, ErrRequired):
		// 422 rather than 403: the request was understood and refused on its
		// content. Document 124 makes this a property of the category, not of
		// who is asking.
		httpx.WriteError(w, r, httpx.Validation(err.Error(),
			map[string]string{"category": "this category cannot be disabled"}))
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, httpx.NotFound("device not found"))
	default:
		httpx.WriteError(w, r, httpx.Internal("could not complete the request").WithCause(err))
	}
}
