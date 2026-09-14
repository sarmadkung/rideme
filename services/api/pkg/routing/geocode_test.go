package routing_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

func newGoogleGeocoder(t *testing.T, stub *googleStub) *routing.GoogleGeocoder {
	t.Helper()
	geocoder, err := routing.NewGoogleGeocoder(testAPIKey,
		routing.WithGoogleBaseURL(stub.server.URL),
		routing.WithGoogleHTTPClient(stub.server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return geocoder
}

const textSearchOK = `{
  "status": "OK",
  "results": [
    {"place_id": "p1", "name": "Liberty Market",
     "formatted_address": "Liberty Market, Gulberg III, Lahore",
     "geometry": {"location": {"lat": 31.5169, "lng": 74.3484}}},
    {"place_id": "p2", "name": "Liberty Chowk",
     "formatted_address": "Liberty Chowk, Gulberg, Lahore",
     "geometry": {"location": {"lat": 31.5155, "lng": 74.3470}}}
  ]
}`

func TestSearchReturnsPlacesACustomerCanRecogniseAndADriverCanFind(t *testing.T) {
	stub := newGoogleStub(t, http.StatusOK, textSearchOK)
	geocoder := newGoogleGeocoder(t, stub)

	places, err := geocoder.Search(context.Background(), "Liberty", gulberg)
	if err != nil {
		t.Fatal(err)
	}
	if len(places) != 2 {
		t.Fatalf("places = %d, want 2", len(places))
	}
	// The name is what a customer recognises; the address is what a driver
	// needs to reach the right one.
	if places[0].Name != "Liberty Market" {
		t.Errorf("name = %q", places[0].Name)
	}
	if !strings.Contains(places[0].Address, "Gulberg") {
		t.Errorf("address = %q, want the full address", places[0].Address)
	}
	if places[0].Point.Lat != 31.5169 {
		t.Errorf("point = %v, want the result's coordinates", places[0].Point)
	}
	if places[0].ProviderID != "p1" {
		t.Errorf("provider id = %q, want it kept", places[0].ProviderID)
	}
}

func TestSearchIsBiasedTowardTheCustomerButNotFilteredToThem(t *testing.T) {
	// "Liberty" matches places in several Pakistani cities, and a customer in
	// Lahore means the one in Lahore — but an airport across town must still
	// be findable, so the location is a bias, not a bound.
	stub := newGoogleStub(t, http.StatusOK, textSearchOK)
	geocoder := newGoogleGeocoder(t, stub)

	if _, err := geocoder.Search(context.Background(), "Liberty", gulberg); err != nil {
		t.Fatal(err)
	}
	if stub.query.Get("location") == "" {
		t.Error("the customer's position was not sent as a bias")
	}
	if stub.query.Get("radius") == "" {
		t.Error("no bias radius was sent")
	}
	// Nothing that would exclude results outright.
	for _, bounding := range []string{"bounds", "strictbounds", "rankby"} {
		if stub.query.Get(bounding) != "" {
			t.Errorf("%s was sent; the bias must not become a filter", bounding)
		}
	}
}

func TestSearchWithoutAPositionStillWorks(t *testing.T) {
	// A customer whose location has not arrived yet can still type an address.
	stub := newGoogleStub(t, http.StatusOK, textSearchOK)
	geocoder := newGoogleGeocoder(t, stub)

	if _, err := geocoder.Search(context.Background(), "Lahore Airport", routing.Point{}); err != nil {
		t.Fatal(err)
	}
	if stub.query.Get("location") != "" {
		t.Error("the zero point was sent as a real bias")
	}
}

func TestAnEmptyQueryIsRefusedWithoutSpendingACall(t *testing.T) {
	stub := newGoogleStub(t, http.StatusOK, textSearchOK)
	geocoder := newGoogleGeocoder(t, stub)

	for _, query := range []string{"", "   ", "\t"} {
		if _, err := geocoder.Search(context.Background(), query, gulberg); !errors.Is(err, routing.ErrEmptyQuery) {
			t.Errorf("Search(%q) = %v, want ErrEmptyQuery", query, err)
		}
	}
	if stub.path != "" {
		t.Error("an empty query was still sent to the provider")
	}
}

func TestFindingNothingIsAnAnswerNotAFault(t *testing.T) {
	// The customer typed something that is not a place. Telling them so is the
	// correct outcome, and it must be distinguishable from the provider being
	// broken.
	stub := newGoogleStub(t, http.StatusOK, `{"status":"ZERO_RESULTS","results":[]}`)
	geocoder := newGoogleGeocoder(t, stub)

	_, err := geocoder.Search(context.Background(), "asdfghjkl", gulberg)
	if !errors.Is(err, routing.ErrNoPlace) {
		t.Fatalf("err = %v, want ErrNoPlace", err)
	}
}

func TestAResultThePlatformCannotRouteToIsDropped(t *testing.T) {
	// It would sit in the list looking selectable and fail at quote time.
	const broken = `{"status":"OK","results":[
	  {"place_id":"bad","name":"Nowhere","formatted_address":"Nowhere",
	   "geometry":{"location":{"lat":999,"lng":74.3}}},
	  {"place_id":"good","name":"Liberty Market","formatted_address":"Liberty Market, Lahore",
	   "geometry":{"location":{"lat":31.5169,"lng":74.3484}}}]}`
	stub := newGoogleStub(t, http.StatusOK, broken)
	geocoder := newGoogleGeocoder(t, stub)

	places, err := geocoder.Search(context.Background(), "Liberty", gulberg)
	if err != nil {
		t.Fatal(err)
	}
	if len(places) != 1 || places[0].ProviderID != "good" {
		t.Errorf("places = %+v, want only the routable one", places)
	}
}

func TestSearchResultsAreCappedForAPhone(t *testing.T) {
	var results []string
	for i := range 30 {
		results = append(results, `{"place_id":"p`+string(rune('a'+i%26))+`","name":"Place",
		  "formatted_address":"Somewhere, Lahore",
		  "geometry":{"location":{"lat":31.5,"lng":74.3}}}`)
	}
	stub := newGoogleStub(t, http.StatusOK,
		`{"status":"OK","results":[`+strings.Join(results, ",")+`]}`)
	geocoder := newGoogleGeocoder(t, stub)

	places, err := geocoder.Search(context.Background(), "Place", gulberg)
	if err != nil {
		t.Fatal(err)
	}
	if len(places) != routing.MaxSearchResults {
		t.Errorf("places = %d, want %d", len(places), routing.MaxSearchResults)
	}
}

func TestReverseKeepsThePointThatWasAskedAbout(t *testing.T) {
	// The customer is standing where they are standing. Moving their pickup to
	// the provider's snapped centroid would be a worse answer than the one
	// they gave.
	const reverseOK = `{"status":"OK","results":[
	  {"place_id":"r1","formatted_address":"12 Main Boulevard, Gulberg III, Lahore"}]}`
	stub := newGoogleStub(t, http.StatusOK, reverseOK)
	geocoder := newGoogleGeocoder(t, stub)

	place, err := geocoder.Reverse(context.Background(), gulberg)
	if err != nil {
		t.Fatal(err)
	}
	if place.Point != gulberg {
		t.Errorf("point = %v, want the point asked about (%v)", place.Point, gulberg)
	}
	// A short name to show, rather than the full address twice.
	if place.Name != "12 Main Boulevard" {
		t.Errorf("name = %q, want the first line", place.Name)
	}
	if place.Address != "12 Main Boulevard, Gulberg III, Lahore" {
		t.Errorf("address = %q", place.Address)
	}
}

func TestReverseRefusesAnImpossiblePointWithoutSpendingACall(t *testing.T) {
	stub := newGoogleStub(t, http.StatusOK, `{"status":"OK","results":[]}`)
	geocoder := newGoogleGeocoder(t, stub)

	if _, err := geocoder.Reverse(context.Background(),
		routing.Point{Lat: 91, Lon: 0}); !errors.Is(err, routing.ErrBadPoint) {
		t.Fatalf("err = %v, want ErrBadPoint", err)
	}
	if stub.path != "" {
		t.Error("an impossible point was still sent to the provider")
	}
}

func TestTheGeocoderKeyNeverReachesAnErrorEither(t *testing.T) {
	// The same guarantee the routing provider makes, asserted separately: the
	// two share one HTTP call precisely so they cannot drift apart on this.
	stub := newGoogleStub(t, http.StatusInternalServerError, `{}`)
	geocoder := newGoogleGeocoder(t, stub)

	_, err := geocoder.Search(context.Background(), "Liberty", gulberg)
	if err == nil {
		t.Fatal("a 500 was accepted")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("the API key leaked into an error: %v", err)
	}
}

func TestTheGeocoderRejectsAnEmptyKeyAtConstruction(t *testing.T) {
	if _, err := routing.NewGoogleGeocoder("  "); !errors.Is(err, routing.ErrNoAPIKey) {
		t.Errorf("err = %v, want ErrNoAPIKey", err)
	}
}

func TestReversePrefersAnAddressOverAPlusCode(t *testing.T) {
	// Google orders reverse results by specificity, and for a point that is not
	// on a building the most specific answer is a Plus Code — "G9C5+5F5". It is
	// precise, correct, and unusable on a screen where a customer is confirming
	// a pickup. This was found by running against the real API, not by reading
	// the documentation.
	const withPlusCode = `{"status":"OK","results":[
	  {"place_id":"pc","formatted_address":"G9C5+5F5, Block N Gulberg III, Lahore","types":["plus_code"]},
	  {"place_id":"st","formatted_address":"12 Main Boulevard, Gulberg III, Lahore","types":["street_address"]}]}`
	stub := newGoogleStub(t, http.StatusOK, withPlusCode)
	geocoder := newGoogleGeocoder(t, stub)

	place, err := geocoder.Reverse(context.Background(), gulberg)
	if err != nil {
		t.Fatal(err)
	}
	if place.Name != "12 Main Boulevard" {
		t.Errorf("name = %q, want the street address rather than the Plus Code", place.Name)
	}
	if place.ProviderID != "st" {
		t.Errorf("provider id = %q, want the street result", place.ProviderID)
	}
}

func TestAPlusCodeIsStillBetterThanNothing(t *testing.T) {
	// Somewhere with no street address at all — a field, a plot — has only the
	// Plus Code, and refusing to answer would be worse than an ugly answer.
	const only = `{"status":"OK","results":[
	  {"place_id":"pc","formatted_address":"G9C5+5F5, Lahore","types":["plus_code"]}]}`
	stub := newGoogleStub(t, http.StatusOK, only)
	geocoder := newGoogleGeocoder(t, stub)

	place, err := geocoder.Reverse(context.Background(), gulberg)
	if err != nil {
		t.Fatal(err)
	}
	if place.ProviderID != "pc" {
		t.Errorf("provider id = %q, want the only result there was", place.ProviderID)
	}
}

func TestAPlusCodeIsRecognisedWithoutTypes(t *testing.T) {
	// Not every response carries the types array.
	const untyped = `{"status":"OK","results":[
	  {"place_id":"pc","formatted_address":"G9C5+5F5, Block N Gulberg III, Lahore"},
	  {"place_id":"st","formatted_address":"12 Main Boulevard, Gulberg III, Lahore"}]}`
	stub := newGoogleStub(t, http.StatusOK, untyped)
	geocoder := newGoogleGeocoder(t, stub)

	place, err := geocoder.Reverse(context.Background(), gulberg)
	if err != nil {
		t.Fatal(err)
	}
	if place.ProviderID != "st" {
		t.Errorf("provider id = %q, want the street result", place.ProviderID)
	}
}
