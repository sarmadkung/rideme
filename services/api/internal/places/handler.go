// Package places is the customer-facing search surface over the geocoding
// boundary (documents 14, 105).
//
// It exists so the map provider's key stays on the server. A client that
// searched Google directly would need a key in its bundle, and a key in a
// bundle is a key anyone can spend.
package places

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

// MaxQueryLength bounds what will be forwarded to the provider.
//
// A search box does not need more, and an unbounded string is a billed call
// somebody else chose the size of.
const MaxQueryLength = 200

type Handler struct {
	geocoder routing.Geocoder
}

// NewHandler returns nil when there is no geocoder, so the routes are not
// registered at all. An endpoint that exists and always fails is worse than
// one that is honestly absent: a client can detect a 404 and fall back to its
// own list of places, but it cannot tell a misconfigured server from a
// customer who typed a nonsense address.
func NewHandler(geocoder routing.Geocoder) *Handler {
	if geocoder == nil {
		return nil
	}
	return &Handler{geocoder: geocoder}
}

func (h *Handler) Routes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	const p = httpx.APIVersionPrefix
	// Authenticated: each call costs money, so it is not left open to anyone
	// who can reach the host.
	mux.Handle("GET "+p+"/places", authenticate(http.HandlerFunc(h.search)))
	mux.Handle("GET "+p+"/places/reverse", authenticate(http.HandlerFunc(h.reverse)))
}

// SearchResponse is the shape returned to a client.
type SearchResponse struct {
	Places []routing.Place `json:"places"`
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if len(query) > MaxQueryLength {
		query = query[:MaxQueryLength]
	}

	// The customer's position, when the client has one. Absent is normal —
	// their first fix may not have arrived — so it biases the search when
	// present and is simply skipped when not.
	near, err := optionalPoint(r, "lat", "lon")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	places, err := h.geocoder.Search(r.Context(), query, near)
	if err != nil {
		httpx.WriteError(w, r, searchError(err))
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, SearchResponse{Places: places})
}

func (h *Handler) reverse(w http.ResponseWriter, r *http.Request) {
	at, err := requiredPoint(r, "lat", "lon")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	place, err := h.geocoder.Reverse(r.Context(), at)
	if err != nil {
		httpx.WriteError(w, r, searchError(err))
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, place)
}

// searchError maps the boundary's errors onto the platform envelope.
//
// The distinction that matters is between "you asked for something that is not
// there" and "we could not ask". A customer shown "no results" for a provider
// outage will retype their address until they give up.
func searchError(err error) error {
	switch {
	case errors.Is(err, routing.ErrEmptyQuery):
		return httpx.Validation("a search needs a query", map[string]string{"q": "required"})
	case errors.Is(err, routing.ErrBadPoint):
		return httpx.Validation("that is not a position on Earth",
			map[string]string{"lat": "out of range", "lon": "out of range"})
	case errors.Is(err, routing.ErrNoPlace):
		return httpx.NotFound("no place matched")
	default:
		// The provider's own message can name a disabled API or a restricted
		// key. That is an operator's problem, not a customer's, and it is
		// already in the logs.
		return httpx.Unavailable("search is unavailable right now")
	}
}

func optionalPoint(r *http.Request, latKey, lonKey string) (routing.Point, error) {
	rawLat, rawLon := r.URL.Query().Get(latKey), r.URL.Query().Get(lonKey)
	if rawLat == "" && rawLon == "" {
		return routing.Point{}, nil
	}
	return parsePoint(rawLat, rawLon, latKey, lonKey)
}

func requiredPoint(r *http.Request, latKey, lonKey string) (routing.Point, error) {
	rawLat, rawLon := r.URL.Query().Get(latKey), r.URL.Query().Get(lonKey)
	if rawLat == "" || rawLon == "" {
		return routing.Point{}, httpx.Validation("a position is required",
			map[string]string{latKey: "required", lonKey: "required"})
	}
	return parsePoint(rawLat, rawLon, latKey, lonKey)
}

func parsePoint(rawLat, rawLon, latKey, lonKey string) (routing.Point, error) {
	details := map[string]string{}
	lat, latErr := strconv.ParseFloat(rawLat, 64)
	if latErr != nil {
		details[latKey] = "must be a number"
	}
	lon, lonErr := strconv.ParseFloat(rawLon, 64)
	if lonErr != nil {
		details[lonKey] = "must be a number"
	}
	if len(details) > 0 {
		return routing.Point{}, httpx.Validation("a position must be two numbers", details)
	}

	point := routing.Point{Lat: lat, Lon: lon}
	if !point.Valid() {
		return routing.Point{}, httpx.Validation("that is not a position on Earth",
			map[string]string{latKey: "out of range", lonKey: "out of range"})
	}
	return point, nil
}
