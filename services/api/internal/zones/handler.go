package zones

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Handler serves the admin zone-management surface (document 143).
type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// Routes registers the documented endpoints, admin-only.
func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	adminOnly := func(fn http.HandlerFunc) http.Handler {
		return authenticate(identity.RequireRole(identity.RoleAdmin, identity.RoleSuperAdmin)(fn))
	}
	mux.Handle("POST "+p+"/admin/zones", adminOnly(h.create))
	mux.Handle("GET "+p+"/admin/zones", adminOnly(h.list))
}

type createZoneBody struct {
	Name         string  `json:"name"`
	City         string  `json:"city,omitempty"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	RadiusMeters int     `json:"radius_meters"`
}

// ZoneResponse is the shape clients receive. Exported because it is part of
// the wire contract and is generated into TypeScript (ADR-007).
type ZoneResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	City         string    `json:"city,omitempty"`
	Latitude     float64   `json:"latitude"`
	Longitude    float64   `json:"longitude"`
	RadiusMeters int       `json:"radius_meters"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

func ToZoneResponse(z Zone) ZoneResponse {
	return ZoneResponse{
		ID: z.ID, Name: z.Name, City: z.City,
		Latitude: z.Lat, Longitude: z.Lon, RadiusMeters: z.RadiusMeters,
		Status: string(z.Status), CreatedAt: z.CreatedAt,
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body createZoneBody
	if !decode(w, r, &body) {
		return
	}
	z, err := h.service.Create(r.Context(), Zone{
		Name: body.Name, City: body.City,
		Lat: body.Latitude, Lon: body.Longitude, RadiusMeters: body.RadiusMeters,
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusCreated, ToZoneResponse(z))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.List(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := make([]ZoneResponse, 0, len(list))
	for _, z := range list {
		out = append(out, ToZoneResponse(z))
	}
	httpx.WriteJSON(w, r, http.StatusOK, out)
}

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
