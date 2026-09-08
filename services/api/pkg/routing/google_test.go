package routing_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

const testAPIKey = "test-key-not-a-real-credential"

// googleStub serves a canned body and records the query it was called with, so
// a test can assert on what was actually asked of the provider.
type googleStub struct {
	server *httptest.Server
	query  url.Values
	path   string
}

func newGoogleStub(t *testing.T, status int, body string) *googleStub {
	t.Helper()
	stub := &googleStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.query = r.URL.Query()
		stub.path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func newGoogleProvider(t *testing.T, stub *googleStub) *routing.GoogleProvider {
	t.Helper()
	provider, err := routing.NewGoogleProvider(testAPIKey,
		routing.WithGoogleBaseURL(stub.server.URL),
		routing.WithGoogleHTTPClient(stub.server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

const directionsOK = `{
  "status": "OK",
  "routes": [{
    "overview_polyline": {"points": "_p~iF~ps|U"},
    "legs": [
      {"distance": {"value": 5200}, "duration": {"value": 600}, "duration_in_traffic": {"value": 780}},
      {"distance": {"value": 1800}, "duration": {"value": 240}, "duration_in_traffic": {"value": 300}}
    ]
  }]
}`

func TestGoogleRouteSumsLegsAndIsLive(t *testing.T) {
	stub := newGoogleStub(t, http.StatusOK, directionsOK)
	provider := newGoogleProvider(t, stub)

	route, err := provider.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}

	if route.DistanceMeters != 7000 {
		t.Errorf("distance = %d, want 7000 (5200 + 1800)", route.DistanceMeters)
	}
	if route.DurationSeconds != 840 {
		t.Errorf("duration = %d, want 840 (600 + 240)", route.DurationSeconds)
	}
	if route.TrafficDurationSeconds != 1080 {
		t.Errorf("traffic duration = %d, want 1080 (780 + 300)", route.TrafficDurationSeconds)
	}
	if route.Confidence != routing.ConfidenceLive {
		t.Errorf("confidence = %q, want live: a measured road route is not an estimate", route.Confidence)
	}
	if len(route.Legs) != 2 {
		t.Fatalf("legs = %d, want 2", len(route.Legs))
	}
	if route.Geometry == "" {
		t.Error("geometry is empty; the overview polyline was dropped")
	}
}

func TestGoogleRouteReportsNoTrafficModelRatherThanADuplicateFigure(t *testing.T) {
	// Route treats a non-zero TrafficDurationSeconds as the more honest
	// duration. Echoing the free-flow figure into it would claim a traffic
	// model that the response never contained.
	const noTraffic = `{"status":"OK","routes":[{"legs":[
	  {"distance":{"value":3000},"duration":{"value":400}}]}]}`
	stub := newGoogleStub(t, http.StatusOK, noTraffic)
	provider := newGoogleProvider(t, stub)

	route, err := provider.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if route.TrafficDurationSeconds != 0 {
		t.Errorf("traffic duration = %d, want 0 when the provider offered none",
			route.TrafficDurationSeconds)
	}
	if route.DurationSeconds != 400 {
		t.Errorf("duration = %d, want 400", route.DurationSeconds)
	}
}

func TestGoogleRefusesTruckRoutesRatherThanServingACarRoute(t *testing.T) {
	// Document 95: "Do not assume a car route is always valid for a truck."
	// Google Directions has no profile that models axle weight or lane bans,
	// so the honest answer is that it cannot answer.
	stub := newGoogleStub(t, http.StatusOK, directionsOK)
	provider := newGoogleProvider(t, stub)

	_, err := provider.Route(context.Background(), lahoreFort, gujranwala,
		routing.Options{Mode: routing.ModeTruck})
	if !errors.Is(err, routing.ErrModeUnsupported) {
		t.Fatalf("err = %v, want ErrModeUnsupported", err)
	}
	if stub.path != "" {
		t.Error("a billed call was made for a mode the provider cannot serve")
	}
}

func TestATruckFallsBackToAnEstimateInsteadOfFailing(t *testing.T) {
	// The refusal above must not strand the booking: Service degrades to the
	// straight-line estimator, and the caller can see it did.
	stub := newGoogleStub(t, http.StatusOK, directionsOK)
	service := routing.NewService(newGoogleProvider(t, stub))

	route, err := service.Route(context.Background(), lahoreFort, gujranwala,
		routing.Options{Mode: routing.ModeTruck})
	if err != nil {
		t.Fatal(err)
	}
	if route.Confidence != routing.ConfidenceEstimated {
		t.Errorf("confidence = %q, want estimated", route.Confidence)
	}
	if route.Provider != "straight-line" {
		t.Errorf("provider = %q, want straight-line", route.Provider)
	}
}

func TestGoogleSelectsTheTwoWheelerProfileForMotorcycles(t *testing.T) {
	stub := newGoogleStub(t, http.StatusOK, directionsOK)
	provider := newGoogleProvider(t, stub)

	if _, err := provider.Route(context.Background(), lahoreFort, gulberg,
		routing.Options{Mode: routing.ModeMotorcycle}); err != nil {
		t.Fatal(err)
	}
	if got := stub.query.Get("mode"); got != "two_wheeler" {
		t.Errorf("mode = %q, want two_wheeler", got)
	}
}

func TestGoogleTreatsAnInBodyFailureAsAFailure(t *testing.T) {
	// Both endpoints answer HTTP 200 for application-level failures. A caller
	// checking only the status code would read a rejected key as an empty
	// result and quietly price the trip off a fallback.
	const denied = `{"status":"REQUEST_DENIED","error_message":"The provided API key is expired."}`
	stub := newGoogleStub(t, http.StatusOK, denied)
	provider := newGoogleProvider(t, stub)

	_, err := provider.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err == nil {
		t.Fatal("a REQUEST_DENIED body was accepted as a route")
	}
	if !strings.Contains(err.Error(), "REQUEST_DENIED") {
		t.Errorf("err = %v, want the status named so an operator can act on it", err)
	}
}

func TestTheAPIKeyNeverReachesAnErrorMessage(t *testing.T) {
	// The key travels as a query parameter, so the request URL is itself a
	// credential. An error that stringifies the URL turns every transport
	// failure into a disclosure in the logs.
	stub := newGoogleStub(t, http.StatusInternalServerError, `{"status":"UNKNOWN_ERROR"}`)
	provider := newGoogleProvider(t, stub)

	_, httpErr := provider.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if httpErr == nil {
		t.Fatal("expected an error for a 500 response")
	}

	// And on a transport failure, where net/http's *url.Error carries the URL.
	closed := newGoogleStub(t, http.StatusOK, directionsOK)
	unreachable := newGoogleProvider(t, closed)
	closed.server.Close()
	_, transportErr := unreachable.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if transportErr == nil {
		t.Fatal("expected an error against a closed server")
	}

	for _, err := range []error{httpErr, transportErr} {
		if strings.Contains(err.Error(), testAPIKey) {
			t.Errorf("the API key leaked into an error: %v", err)
		}
	}
}

const matrixOK = `{
  "status": "OK",
  "rows": [
    {"elements": [
      {"status": "OK", "distance": {"value": 1200}, "duration": {"value": 180}},
      {"status": "ZERO_RESULTS"}
    ]},
    {"elements": [
      {"status": "OK", "distance": {"value": 3400}, "duration": {"value": 420}},
      {"status": "OK", "distance": {"value": 900},  "duration": {"value": 120}}
    ]}
  ]
}`

func TestGoogleMatrixKeepsReachablePairsAndDropsTheRest(t *testing.T) {
	// One unreachable driver must not discard the grid; dispatch still has to
	// rank everybody else.
	stub := newGoogleStub(t, http.StatusOK, matrixOK)
	provider := newGoogleProvider(t, stub)

	matrix, err := provider.Matrix(context.Background(),
		[]routing.Point{lahoreFort, gulberg},
		[]routing.Point{gulberg, gujranwala},
		routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix.Entries) != 3 {
		t.Fatalf("entries = %d, want 3 (the ZERO_RESULTS cell is dropped)", len(matrix.Entries))
	}
	for _, entry := range matrix.Entries {
		if entry.Confidence != routing.ConfidenceLive {
			t.Errorf("entry %v confidence = %q, want live", entry, entry.Confidence)
		}
	}
	// Origin 1 is nearest to destination 1 at 120 seconds — the reduction
	// dispatch's eta_score term performs.
	origin, duration, ok := matrix.Best(1)
	if !ok || origin != 1 || duration != 120 {
		t.Errorf("Best(1) = (%d, %d, %v), want (1, 120, true)", origin, duration, ok)
	}
}

func TestGoogleRefusesAMatrixLargerThanItCanBill(t *testing.T) {
	// Splitting it would multiply a billed call without the caller asking
	// (document 104). Refusing lets Service answer from the estimator instead.
	stub := newGoogleStub(t, http.StatusOK, matrixOK)
	provider := newGoogleProvider(t, stub)

	origins := make([]routing.Point, 26)
	for i := range origins {
		origins[i] = gulberg
	}

	_, err := provider.Matrix(context.Background(), origins,
		[]routing.Point{lahoreFort}, routing.Options{})
	if !errors.Is(err, routing.ErrMatrixTooLarge) {
		t.Fatalf("err = %v, want ErrMatrixTooLarge", err)
	}
	if stub.path != "" {
		t.Error("an oversized matrix was sent to the provider anyway")
	}
}

func TestGoogleRejectsAnEmptyKeyAtConstruction(t *testing.T) {
	// Failing here beats failing on every request at runtime.
	if _, err := routing.NewGoogleProvider("   "); !errors.Is(err, routing.ErrNoAPIKey) {
		t.Errorf("err = %v, want ErrNoAPIKey", err)
	}
}

func TestGoogleValidatesCoordinatesBeforeSpendingACall(t *testing.T) {
	stub := newGoogleStub(t, http.StatusOK, directionsOK)
	provider := newGoogleProvider(t, stub)

	_, err := provider.Route(context.Background(),
		routing.Point{Lat: 91, Lon: 0}, gulberg, routing.Options{})
	if !errors.Is(err, routing.ErrBadPoint) {
		t.Fatalf("err = %v, want ErrBadPoint", err)
	}
	if stub.path != "" {
		t.Error("an invalid coordinate was still sent to the provider")
	}
}
