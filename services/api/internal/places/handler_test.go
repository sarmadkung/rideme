package places_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sarmadkung/rideme/services/api/internal/places"
	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

// stubGeocoder answers with whatever the test sets, and records the call.
type stubGeocoder struct {
	places  []routing.Place
	place   routing.Place
	err     error
	query   string
	near    routing.Point
	at      routing.Point
	calls   int
	reverse int
}

func (g *stubGeocoder) Name() string { return "stub" }

func (g *stubGeocoder) Search(_ context.Context, query string, near routing.Point) ([]routing.Place, error) {
	g.calls++
	g.query, g.near = query, near
	return g.places, g.err
}

func (g *stubGeocoder) Reverse(_ context.Context, at routing.Point) (routing.Place, error) {
	g.reverse++
	g.at = at
	return g.place, g.err
}

func aPlace() routing.Place {
	return routing.Place{
		ProviderID: "p1",
		Name:       "Liberty Market",
		Address:    "Liberty Market, Gulberg III, Lahore",
		Point:      routing.Point{Lat: 31.5169, Lon: 74.3484},
	}
}

// serve builds the routes with authentication stubbed out; whether the
// middleware works is identity's concern, not this package's.
func serve(t *testing.T, geocoder routing.Geocoder) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	handler := places.NewHandler(geocoder)
	if handler == nil {
		return mux
	}
	handler.Routes(mux, func(next http.Handler) http.Handler { return next })
	return mux
}

func get(t *testing.T, mux *http.ServeMux, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestSearchReturnsPlaces(t *testing.T) {
	geocoder := &stubGeocoder{places: []routing.Place{aPlace()}}
	response := get(t, serve(t, geocoder), "/api/v1/places?q=Liberty&lat=31.5204&lon=74.3587")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	var body places.SearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Places) != 1 || body.Places[0].Name != "Liberty Market" {
		t.Fatalf("places = %+v", body.Places)
	}
	if geocoder.query != "Liberty" {
		t.Errorf("query = %q", geocoder.query)
	}
	if geocoder.near.Lat != 31.5204 {
		t.Errorf("near = %v, want the customer's position as a bias", geocoder.near)
	}
}

func TestSearchWorksWithoutTheCustomersPosition(t *testing.T) {
	// A customer whose first fix has not arrived can still type an address.
	geocoder := &stubGeocoder{places: []routing.Place{aPlace()}}
	response := get(t, serve(t, geocoder), "/api/v1/places?q=Lahore+Airport")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	if geocoder.near != (routing.Point{}) {
		t.Errorf("near = %v, want the zero point", geocoder.near)
	}
}

func TestAnOutageIsNotReportedAsAnEmptyResult(t *testing.T) {
	// A customer shown "no results" for a provider outage retypes their
	// address until they give up. The two must not look the same.
	notFound := &stubGeocoder{err: routing.ErrNoPlace}
	if code := get(t, serve(t, notFound), "/api/v1/places?q=asdfgh").Code; code != http.StatusNotFound {
		t.Errorf("no match gave %d, want 404", code)
	}

	broken := &stubGeocoder{err: errors.New("provider exploded")}
	if code := get(t, serve(t, broken), "/api/v1/places?q=Liberty").Code; code != http.StatusServiceUnavailable {
		t.Errorf("provider failure gave %d, want 503", code)
	}
}

func TestTheProvidersOwnMessageIsNotShownToACustomer(t *testing.T) {
	// It can name a disabled API or a restricted key: an operator's problem,
	// already in the logs, and not something to print on a phone.
	geocoder := &stubGeocoder{err: errors.New("REQUEST_DENIED: referer restriction on key")}
	response := get(t, serve(t, geocoder), "/api/v1/places?q=Liberty")

	if body := response.Body.String(); strings.Contains(body, "REQUEST_DENIED") || strings.Contains(body, "referer") {
		t.Errorf("the provider's message leaked to the client: %s", body)
	}
}

func TestAnEmptyQueryIsRejectedBeforeTheProvider(t *testing.T) {
	geocoder := &stubGeocoder{err: routing.ErrEmptyQuery}
	response := get(t, serve(t, geocoder), "/api/v1/places?q=")
	if response.Code != http.StatusBadRequest && response.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want a validation failure; body %s", response.Code, response.Body)
	}
}

func TestAMalformedPositionIsAValidationFailureNotACrash(t *testing.T) {
	geocoder := &stubGeocoder{places: []routing.Place{aPlace()}}
	mux := serve(t, geocoder)

	for _, target := range []string{
		"/api/v1/places?q=Liberty&lat=abc&lon=74.3",
		"/api/v1/places?q=Liberty&lat=91&lon=74.3",
		"/api/v1/places/reverse?lat=31.5&lon=999",
	} {
		response := get(t, mux, target)
		if response.Code == http.StatusOK || response.Code >= 500 {
			t.Errorf("%s gave %d, want a client error", target, response.Code)
		}
	}
	if geocoder.calls != 0 || geocoder.reverse != 0 {
		t.Error("a malformed position was still sent to a billed provider")
	}
}

func TestReverseNamesAPoint(t *testing.T) {
	geocoder := &stubGeocoder{place: aPlace()}
	response := get(t, serve(t, geocoder), "/api/v1/places/reverse?lat=31.5169&lon=74.3484")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	var place routing.Place
	if err := json.Unmarshal(response.Body.Bytes(), &place); err != nil {
		t.Fatal(err)
	}
	if place.Name != "Liberty Market" {
		t.Errorf("name = %q", place.Name)
	}
	if geocoder.at.Lat != 31.5169 {
		t.Errorf("at = %v", geocoder.at)
	}
}

func TestReverseRequiresAPosition(t *testing.T) {
	geocoder := &stubGeocoder{place: aPlace()}
	response := get(t, serve(t, geocoder), "/api/v1/places/reverse")
	if response.Code == http.StatusOK {
		t.Error("reverse answered without being told where")
	}
}

func TestAnUnboundedQueryIsTruncatedRatherThanForwarded(t *testing.T) {
	// An unbounded string is a billed call somebody else chose the size of.
	long := ""
	for range places.MaxQueryLength + 500 {
		long += "a"
	}
	geocoder := &stubGeocoder{places: []routing.Place{aPlace()}}
	get(t, serve(t, geocoder), "/api/v1/places?q="+long)

	if len(geocoder.query) != places.MaxQueryLength {
		t.Errorf("forwarded %d characters, want it capped at %d",
			len(geocoder.query), places.MaxQueryLength)
	}
}

func TestWithNoGeocoderTheRoutesDoNotExist(t *testing.T) {
	// An endpoint that exists and always fails is worse than an honestly
	// absent one: a client can detect a 404 and fall back to its own list, but
	// cannot tell a misconfigured server from a nonsense address.
	if handler := places.NewHandler(nil); handler != nil {
		t.Fatal("a handler was built with no geocoder")
	}
	if code := get(t, serve(t, nil), "/api/v1/places?q=Liberty").Code; code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}
