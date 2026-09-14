package routing

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// Place is somewhere a customer can be picked up from or taken to.
//
// The name and the address are separate because they answer different
// questions: a customer recognises "Liberty Market", and a driver needs
// "Liberty Market, Gulberg III, Lahore" to find the right one.
type Place struct {
	// ProviderID is the provider's own identifier, kept so a later lookup can
	// ask about this exact place rather than searching again for its name.
	ProviderID string `json:"provider_id,omitempty"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	Point      Point  `json:"point"`
}

// Geocoder turns text into places and points back into names.
//
// Separate from Provider because routing.go's boundary is deliberately narrow:
// "a provider that also geocodes implements Geocoder separately." Routing and
// geocoding are billed as different products and can be bought from different
// vendors — OSRM routes well and geocodes not at all.
type Geocoder interface {
	Name() string
	// Search finds places matching free text, biased toward near. A caller
	// with no meaningful location passes the zero Point.
	Search(ctx context.Context, query string, near Point) ([]Place, error)
	// Reverse names the place at a point.
	Reverse(ctx context.Context, at Point) (Place, error)
}

var (
	// ErrEmptyQuery means there is nothing to search for. Sending it to the
	// provider would spend a billed call to be told the same thing.
	ErrEmptyQuery = errors.New("routing: a search needs a query")
	// ErrNoPlace means the provider found nothing. It is not a failure of the
	// platform and must not be presented as one.
	ErrNoPlace = errors.New("routing: no place found")
)

// SearchBiasRadiusM is how far around `near` results are preferred.
//
// A city's width. "Liberty" matches places in several Pakistani cities, and a
// customer in Lahore means the one in Lahore — but the bias is a preference,
// not a filter, so an airport across town is still findable.
const SearchBiasRadiusM = 30000

// MaxSearchResults caps what is returned to a client.
//
// A search box shows a handful. Returning sixty would spend bandwidth on a
// phone that will display five of them.
const MaxSearchResults = 8

// GoogleGeocoder is Geocoder over the Google Maps Platform.
//
// Search uses Text Search rather than Autocomplete. Autocomplete is the better
// as-you-type experience, but it returns place identifiers without
// coordinates, so every selection costs a second Place Details call. Text
// Search answers with the name, the address and the point in one call, and one
// billed call per search is the cost this platform can defend today.
type GoogleGeocoder struct {
	google
}

func NewGoogleGeocoder(apiKey string, opts ...GoogleOption) (*GoogleGeocoder, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrNoAPIKey
	}
	return &GoogleGeocoder{google: newGoogle(apiKey, opts...)}, nil
}

func (g *GoogleGeocoder) Name() string { return "google" }

func (g *GoogleGeocoder) Search(ctx context.Context, query string, near Point) ([]Place, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyQuery
	}

	values := url.Values{}
	values.Set("query", query)
	if near.Valid() && (near.Lat != 0 || near.Lon != 0) {
		values.Set("location", formatPoint(near))
		values.Set("radius", strconv.Itoa(SearchBiasRadiusM))
	}

	var payload googlePlacesResponse
	if err := g.get(ctx, "place/textsearch/json", values, &payload); err != nil {
		return nil, err
	}
	// ZERO_RESULTS is an answer, not a fault: the customer typed something
	// that is not a place, and telling them so is the correct outcome.
	if payload.Status == "ZERO_RESULTS" {
		return nil, ErrNoPlace
	}
	if err := googleStatusError("textsearch", payload.Status, payload.ErrorMessage); err != nil {
		return nil, err
	}

	places := make([]Place, 0, len(payload.Results))
	for _, result := range payload.Results {
		point := Point{Lat: result.Geometry.Location.Lat, Lon: result.Geometry.Location.Lng}
		// A result the platform cannot route to is worse than one fewer
		// result: it would sit in the list and fail at quote time.
		if !point.Valid() {
			continue
		}
		places = append(places, Place{
			ProviderID: result.PlaceID,
			Name:       result.Name,
			Address:    result.FormattedAddress,
			Point:      point,
		})
		if len(places) == MaxSearchResults {
			break
		}
	}
	if len(places) == 0 {
		return nil, ErrNoPlace
	}
	return places, nil
}

func (g *GoogleGeocoder) Reverse(ctx context.Context, at Point) (Place, error) {
	if !at.Valid() {
		return Place{}, ErrBadPoint
	}

	values := url.Values{}
	values.Set("latlng", formatPoint(at))

	var payload googleGeocodeResponse
	if err := g.get(ctx, "geocode/json", values, &payload); err != nil {
		return Place{}, err
	}
	if payload.Status == "ZERO_RESULTS" {
		return Place{}, ErrNoPlace
	}
	if err := googleStatusError("geocode", payload.Status, payload.ErrorMessage); err != nil {
		return Place{}, err
	}
	if len(payload.Results) == 0 {
		return Place{}, ErrNoPlace
	}

	first := preferNamedResult(payload.Results)
	// Reverse geocoding has no name, only an address. The first line stands in
	// for one so a caller has something short to show, rather than repeating
	// the full address twice in a UI.
	name := first.FormattedAddress
	if comma := strings.Index(name, ","); comma > 0 {
		name = name[:comma]
	}
	return Place{
		ProviderID: first.PlaceID,
		Name:       name,
		Address:    first.FormattedAddress,
		// The point that was asked about, not the provider's snapped
		// centroid: the customer is standing where they are standing, and
		// moving their pickup by a block to match a building record would be
		// a worse answer than the one they gave.
		Point: at,
	}, nil
}

var _ Geocoder = (*GoogleGeocoder)(nil)

// --- wire format -------------------------------------------------------------

type googleLatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type googlePlacesResponse struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	Results      []struct {
		PlaceID          string `json:"place_id"`
		Name             string `json:"name"`
		FormattedAddress string `json:"formatted_address"`
		Geometry         struct {
			Location googleLatLng `json:"location"`
		} `json:"geometry"`
	} `json:"results"`
}

type googleGeocodeResult struct {
	PlaceID          string   `json:"place_id"`
	FormattedAddress string   `json:"formatted_address"`
	Types            []string `json:"types"`
}

type googleGeocodeResponse struct {
	Status       string                `json:"status"`
	ErrorMessage string                `json:"error_message"`
	Results      []googleGeocodeResult `json:"results"`
}

// preferNamedResult picks a result a person would recognise.
//
// Google orders reverse-geocode results by specificity, and the most specific
// answer for a point that is not on a building is a Plus Code — "G9C5+5F5".
// It is a precise, correct, and completely unusable thing to show a customer
// confirming a pickup. A street address one rung less specific is the better
// answer, and the Plus Code is kept only when it is the sole one.
func preferNamedResult(results []googleGeocodeResult) googleGeocodeResult {
	for _, result := range results {
		if !isPlusCode(result) {
			return result
		}
	}
	return results[0]
}

func isPlusCode(result googleGeocodeResult) bool {
	for _, t := range result.Types {
		if t == "plus_code" {
			return true
		}
	}
	// Some responses carry no types. A leading segment holding a "+" is the
	// shape of a Plus Code and nothing else Google returns looks like it.
	head := result.FormattedAddress
	if comma := strings.Index(head, ","); comma > 0 {
		head = head[:comma]
	}
	return strings.Contains(head, "+")
}
