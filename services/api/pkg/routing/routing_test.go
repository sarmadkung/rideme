package routing_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

var (
	lahoreFort = routing.Point{Lat: 31.5880, Lon: 74.3150}
	gulberg    = routing.Point{Lat: 31.5204, Lon: 74.3587}
	gujranwala = routing.Point{Lat: 32.1877, Lon: 74.1945}
)

func TestHaversineMatchesKnownDistances(t *testing.T) {
	// Lahore Fort to Gulberg is roughly 8.4 km straight line.
	if d := routing.HaversineMeters(lahoreFort, gulberg); math.Abs(d-8400) > 500 {
		t.Errorf("Lahore Fort → Gulberg = %.0fm, want ~8400m", d)
	}
	// Lahore to Gujranwala is roughly 67 km.
	if d := routing.HaversineMeters(lahoreFort, gujranwala); math.Abs(d-67000) > 3000 {
		t.Errorf("Lahore → Gujranwala = %.0fm, want ~67000m", d)
	}
	if d := routing.HaversineMeters(gulberg, gulberg); d != 0 {
		t.Errorf("distance to self = %v, want 0", d)
	}
	// Symmetry: a route measured backwards is the same length.
	if a, b := routing.HaversineMeters(lahoreFort, gulberg), routing.HaversineMeters(gulberg, lahoreFort); math.Abs(a-b) > 0.001 {
		t.Errorf("not symmetric: %v vs %v", a, b)
	}
}

func TestModeIsChosenByVehicleType(t *testing.T) {
	// Document 95: "Do not assume a car route is always valid for a truck."
	cases := map[string]routing.Mode{
		"MOTORCYCLE": routing.ModeMotorcycle,
		"CAR":        routing.ModeDriving,
		"RICKSHAW":   routing.ModeDriving,
		"TRUCK":      routing.ModeTruck,
		"MAZDA":      routing.ModeTruck,
		"SHEHZORE":   routing.ModeTruck,
	}
	for vehicleType, want := range cases {
		if got := routing.ModeForVehicleType(vehicleType); got != want {
			t.Errorf("%s -> %s, want %s", vehicleType, got, want)
		}
	}
}

func TestEstimatesAreNeverPresentedAsExact(t *testing.T) {
	// Document 96: "Never present a fallback as exact." A caller that cannot
	// tell a live route from a guess will show both as an arrival time.
	service := routing.NewService() // no providers at all
	route, err := service.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if route.Confidence != routing.ConfidenceEstimated {
		t.Fatalf("confidence = %s, want estimated", route.Confidence)
	}
	if route.DistanceMeters <= 0 || route.DurationSeconds <= 0 {
		t.Fatalf("estimate produced nothing usable: %+v", route)
	}
}

func TestRoadDistanceExceedsStraightLine(t *testing.T) {
	// Roads are not straight. An estimate that returns the great-circle
	// distance under-quotes every fare that uses it.
	service := routing.NewService()
	route, err := service.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	direct := routing.HaversineMeters(lahoreFort, gulberg)
	if float64(route.DistanceMeters) <= direct {
		t.Fatalf("road distance %d is not greater than straight line %.0f", route.DistanceMeters, direct)
	}
}

func TestSlowerModesTakeLonger(t *testing.T) {
	service := routing.NewService()
	ctx := context.Background()

	bike, err := service.EstimateETA(ctx, lahoreFort, gujranwala, routing.Options{Mode: routing.ModeMotorcycle})
	if err != nil {
		t.Fatal(err)
	}
	truck, err := service.EstimateETA(ctx, lahoreFort, gujranwala, routing.Options{Mode: routing.ModeTruck})
	if err != nil {
		t.Fatal(err)
	}
	if truck.DurationSeconds <= bike.DurationSeconds {
		t.Fatalf("truck (%ds) is not slower than motorcycle (%ds)", truck.DurationSeconds, bike.DurationSeconds)
	}
	// Distance does not change with the vehicle.
	if truck.DistanceMeters != bike.DistanceMeters {
		t.Errorf("distance differed by mode: %d vs %d", truck.DistanceMeters, bike.DistanceMeters)
	}
}

func TestBadCoordinatesAreRefused(t *testing.T) {
	service := routing.NewService()
	if _, err := service.Route(context.Background(),
		routing.Point{Lat: 91, Lon: 0}, gulberg, routing.Options{}); !errors.Is(err, routing.ErrBadPoint) {
		t.Fatalf("want ErrBadPoint, got %v", err)
	}
}

func TestMatrixCoversEveryPairAndFindsTheNearest(t *testing.T) {
	// This is the Drivers × Pickup matrix document 96 specifies for dispatch,
	// and the reduction dispatch's eta_score performs on it.
	service := routing.NewService()
	drivers := []routing.Point{gujranwala, gulberg, lahoreFort}
	pickup := []routing.Point{gulberg}

	matrix, err := service.Matrix(context.Background(), drivers, pickup, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix.Entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(matrix.Entries))
	}

	nearest, duration, ok := matrix.Best(0)
	if !ok {
		t.Fatal("no nearest origin found")
	}
	// Index 1 is Gulberg itself — zero distance from the pickup.
	if nearest != 1 {
		t.Fatalf("nearest origin = %d, want 1", nearest)
	}
	if duration != 0 {
		t.Fatalf("duration to self = %d, want 0", duration)
	}
}

func TestEmptyMatrixIsRefused(t *testing.T) {
	service := routing.NewService()
	if _, err := service.Matrix(context.Background(), nil, []routing.Point{gulberg}, routing.Options{}); !errors.Is(err, routing.ErrEmptyMatrix) {
		t.Fatalf("want ErrEmptyMatrix, got %v", err)
	}
}

// failingProvider stands in for a provider outage.
type failingProvider struct{ calls int }

func (f *failingProvider) Name() string    { return "failing" }
func (f *failingProvider) Version() string { return "1" }
func (f *failingProvider) Route(context.Context, routing.Point, routing.Point, routing.Options) (routing.Route, error) {
	f.calls++
	return routing.Route{}, errors.New("provider is down")
}
func (f *failingProvider) Matrix(context.Context, []routing.Point, []routing.Point, routing.Options) (routing.Matrix, error) {
	f.calls++
	return routing.Matrix{}, errors.New("provider is down")
}

// fixedProvider returns a known live answer.
type fixedProvider struct{}

func (fixedProvider) Name() string    { return "fixed" }
func (fixedProvider) Version() string { return "2" }
func (fixedProvider) Route(context.Context, routing.Point, routing.Point, routing.Options) (routing.Route, error) {
	return routing.Route{DistanceMeters: 12345, DurationSeconds: 600, TrafficDurationSeconds: 900}, nil
}
func (fixedProvider) Matrix(_ context.Context, origins, destinations []routing.Point, _ routing.Options) (routing.Matrix, error) {
	return routing.Matrix{Entries: []routing.MatrixEntry{{DurationSeconds: 42}}}, nil
}

func TestAFailingProviderFallsThroughToTheNext(t *testing.T) {
	failing := &failingProvider{}
	service := routing.NewService(failing, fixedProvider{})

	route, err := service.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if failing.calls != 1 {
		t.Fatalf("the first provider was called %d times", failing.calls)
	}
	if route.Provider != "fixed" || route.DistanceMeters != 12345 {
		t.Fatalf("fallback did not reach the second provider: %+v", route)
	}
	// A live answer must be labelled live, and stamped with who gave it.
	if route.Confidence != routing.ConfidenceLive || route.ProviderVersion != "2" {
		t.Fatalf("provenance was lost: %+v", route)
	}
}

func TestEveryProviderFailingStillProducesAnEstimate(t *testing.T) {
	// Dispatch ranking by approximate distance beats not dispatching at all.
	service := routing.NewService(&failingProvider{}, &failingProvider{})
	route, err := service.Route(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatalf("a total provider outage failed the request: %v", err)
	}
	if route.Confidence != routing.ConfidenceEstimated {
		t.Fatalf("confidence = %s, want estimated", route.Confidence)
	}
}

func TestETAPrefersTrafficDurationWhenTheProviderHasOne(t *testing.T) {
	// Reporting free-flow time when the provider knows about traffic promises
	// an arrival the driver cannot make.
	service := routing.NewService(fixedProvider{})
	eta, err := service.EstimateETA(context.Background(), lahoreFort, gulberg, routing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if eta.DurationSeconds != 900 {
		t.Fatalf("duration = %d, want the 900s traffic duration", eta.DurationSeconds)
	}
}
