// Package routing is the platform's maps, routing and ETA boundary — CAP-2's
// Phase 6 increment (documents 93, 94, 95, 96).
//
// It is an interface and one provider, not a mapping platform. Document 95's
// objective is the whole requirement: "Prevent routing-provider lock-in and
// make routing replaceable", with business logic receiving normalized routes
// regardless of provider.
//
// The boundary is created now because Phase 7 needs distance and duration for
// the fare terms, and Phase 8 needs a Drivers × Pickup matrix for three of the
// nine scoring terms in document 05. Building it per-caller instead would put
// a provider SDK inside the pricing engine.
package routing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// Point is a WGS-84 coordinate.
type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func (p Point) Valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
}

// Mode is the routing profile. Document 95: "Do not assume a car route is
// always valid for a truck" — a truck barred from a lane needs a different
// route, not the same one with a longer duration.
type Mode string

const (
	ModeDriving    Mode = "driving"
	ModeMotorcycle Mode = "motorcycle"
	ModeTruck      Mode = "truck"
)

// ModeForVehicleType maps the platform's vehicle taxonomy onto routing modes.
func ModeForVehicleType(vehicleType string) Mode {
	switch vehicleType {
	case "MOTORCYCLE":
		return ModeMotorcycle
	case "MAZDA", "TRUCK", "SHEHZORE":
		return ModeTruck
	default:
		return ModeDriving
	}
}

// Confidence records how a result was obtained.
//
// Document 96: "Never present a fallback as exact." A caller that cannot tell
// a live route from a straight-line estimate will show both to a customer as
// an arrival time, and one of them will be wrong in a way nobody can explain.
type Confidence string

const (
	// ConfidenceLive came from a routing provider.
	ConfidenceLive Confidence = "live"
	// ConfidenceCached came from a stored result that is still fresh enough.
	ConfidenceCached Confidence = "cached"
	// ConfidenceEstimated is a geometric approximation. Usable for ranking
	// candidates; not usable as a promise to a customer.
	ConfidenceEstimated Confidence = "estimated"
)

// Leg is one segment of a route.
type Leg struct {
	DistanceMeters  int64 `json:"distance_meters"`
	DurationSeconds int64 `json:"duration_seconds"`
}

// Route is the normalized shape every provider is mapped onto (document 95).
type Route struct {
	DistanceMeters  int64 `json:"distance_meters"`
	DurationSeconds int64 `json:"duration_seconds"`
	// TrafficDurationSeconds is zero when the provider offers no traffic model.
	TrafficDurationSeconds int64      `json:"traffic_duration_seconds,omitempty"`
	Geometry               string     `json:"geometry,omitempty"`
	Legs                   []Leg      `json:"legs,omitempty"`
	Provider               string     `json:"provider"`
	ProviderVersion        string     `json:"provider_version"`
	Confidence             Confidence `json:"confidence"`
}

// ETA is a travel-time estimate.
type ETA struct {
	DurationSeconds int64      `json:"duration_seconds"`
	DistanceMeters  int64      `json:"distance_meters"`
	Confidence      Confidence `json:"confidence"`
	Provider        string     `json:"provider"`
}

// ArrivesAt is the wall-clock arrival implied by the estimate.
func (e ETA) ArrivesAt(from time.Time) time.Time {
	return from.Add(time.Duration(e.DurationSeconds) * time.Second)
}

// MatrixEntry is one origin-to-destination cell.
type MatrixEntry struct {
	OriginIndex      int
	DestinationIndex int
	DistanceMeters   int64
	DurationSeconds  int64
	Confidence       Confidence
}

// Matrix is a Drivers × Pickup or Stops × Stops result (document 96).
type Matrix struct {
	Entries  []MatrixEntry
	Provider string
}

// Best returns the origin index with the shortest duration to a destination.
// This is what dispatch's eta_score term reduces to.
func (m Matrix) Best(destinationIndex int) (originIndex int, duration int64, ok bool) {
	duration = math.MaxInt64
	for _, e := range m.Entries {
		if e.DestinationIndex == destinationIndex && e.DurationSeconds < duration {
			originIndex, duration, ok = e.OriginIndex, e.DurationSeconds, true
		}
	}
	return originIndex, duration, ok
}

// Options are per-request routing parameters.
type Options struct {
	Mode Mode
	// DepartAt allows a scheduled booking to be routed for its departure time
	// rather than for now.
	DepartAt time.Time
}

// Provider is a routing backend. Document 95's three operations, and nothing
// wider: a provider that also geocodes implements Geocoder separately.
type Provider interface {
	Name() string
	Version() string
	Route(ctx context.Context, origin, destination Point, opts Options) (Route, error)
	Matrix(ctx context.Context, origins, destinations []Point, opts Options) (Matrix, error)
}

var (
	ErrNoProvider  = errors.New("routing: no provider configured")
	ErrBadPoint    = errors.New("routing: coordinate is out of range")
	ErrEmptyMatrix = errors.New("routing: a matrix needs origins and destinations")
)

// Service routes through the configured providers, falling back in order.
//
// Document 95 permits a configured fallback and warns: "Do not silently mix
// incompatible routing assumptions." Fallback here is between providers
// answering the *same* mode; it never quietly downgrades a truck route to a car
// route. What it does downgrade is confidence, which the caller can see.
type Service struct {
	providers []Provider
	estimator *StraightLineProvider
}

// NewService builds a routing service. The straight-line estimator is always
// the last resort so that dispatch can still rank candidates when every
// provider is down — ranking by approximate distance beats not dispatching.
func NewService(providers ...Provider) *Service {
	return &Service{providers: providers, estimator: NewStraightLineProvider()}
}

func (s *Service) Route(ctx context.Context, origin, destination Point, opts Options) (Route, error) {
	if !origin.Valid() || !destination.Valid() {
		return Route{}, ErrBadPoint
	}
	if opts.Mode == "" {
		opts.Mode = ModeDriving
	}
	var errs []error
	for _, provider := range s.providers {
		route, err := provider.Route(ctx, origin, destination, opts)
		if err == nil {
			route.Provider, route.ProviderVersion = provider.Name(), provider.Version()
			if route.Confidence == "" {
				route.Confidence = ConfidenceLive
			}
			return route, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", provider.Name(), err))
	}
	// Degrade clearly (document 96) rather than failing the booking.
	route, err := s.estimator.Route(ctx, origin, destination, opts)
	if err != nil {
		return Route{}, errors.Join(append(errs, err)...)
	}
	return route, nil
}

func (s *Service) Matrix(ctx context.Context, origins, destinations []Point, opts Options) (Matrix, error) {
	if len(origins) == 0 || len(destinations) == 0 {
		return Matrix{}, ErrEmptyMatrix
	}
	for _, p := range append(append([]Point{}, origins...), destinations...) {
		if !p.Valid() {
			return Matrix{}, ErrBadPoint
		}
	}
	if opts.Mode == "" {
		opts.Mode = ModeDriving
	}
	for _, provider := range s.providers {
		matrix, err := provider.Matrix(ctx, origins, destinations, opts)
		if err == nil {
			matrix.Provider = provider.Name()
			return matrix, nil
		}
	}
	return s.estimator.Matrix(ctx, origins, destinations, opts)
}

// EstimateETA is the third operation document 95 names.
func (s *Service) EstimateETA(ctx context.Context, origin, destination Point, opts Options) (ETA, error) {
	route, err := s.Route(ctx, origin, destination, opts)
	if err != nil {
		return ETA{}, err
	}
	duration := route.DurationSeconds
	// Traffic duration is the honest answer when the provider has one.
	if route.TrafficDurationSeconds > 0 {
		duration = route.TrafficDurationSeconds
	}
	return ETA{
		DurationSeconds: duration,
		DistanceMeters:  route.DistanceMeters,
		Confidence:      route.Confidence,
		Provider:        route.Provider,
	}, nil
}

// --- straight-line provider --------------------------------------------------

// StraightLineProvider estimates from great-circle distance and a per-mode
// average speed.
//
// It is not a routing provider and does not pretend to be one: every result it
// returns is marked ConfidenceEstimated. It exists so the platform has a
// working routing boundary before a provider contract is signed, and so
// dispatch can still rank candidates during a provider outage — a distance
// that is roughly right orders drivers correctly even when it cannot promise
// an arrival time.
type StraightLineProvider struct {
	// DetourFactor converts great-circle distance into plausible road
	// distance. Roads are not straight; 1.3 is the usual rule of thumb for
	// dense urban networks.
	DetourFactor float64
	SpeedsKPH    map[Mode]float64
}

func NewStraightLineProvider() *StraightLineProvider {
	return &StraightLineProvider{
		DetourFactor: 1.3,
		// Deliberately conservative city speeds. These are engineering
		// defaults, not measurements; document 96 expects a historical model
		// to replace them once there is history to model.
		SpeedsKPH: map[Mode]float64{
			ModeMotorcycle: 25,
			ModeDriving:    20,
			ModeTruck:      15,
		},
	}
}

func (p *StraightLineProvider) Name() string    { return "straight-line" }
func (p *StraightLineProvider) Version() string { return "1" }

func (p *StraightLineProvider) Route(_ context.Context, origin, destination Point, opts Options) (Route, error) {
	if !origin.Valid() || !destination.Valid() {
		return Route{}, ErrBadPoint
	}
	meters, seconds := p.estimate(origin, destination, opts.Mode)
	return Route{
		DistanceMeters:  meters,
		DurationSeconds: seconds,
		Provider:        p.Name(),
		ProviderVersion: p.Version(),
		Confidence:      ConfidenceEstimated,
	}, nil
}

func (p *StraightLineProvider) Matrix(_ context.Context, origins, destinations []Point, opts Options) (Matrix, error) {
	if len(origins) == 0 || len(destinations) == 0 {
		return Matrix{}, ErrEmptyMatrix
	}
	entries := make([]MatrixEntry, 0, len(origins)*len(destinations))
	for i, origin := range origins {
		for j, destination := range destinations {
			meters, seconds := p.estimate(origin, destination, opts.Mode)
			entries = append(entries, MatrixEntry{
				OriginIndex: i, DestinationIndex: j,
				DistanceMeters: meters, DurationSeconds: seconds,
				Confidence: ConfidenceEstimated,
			})
		}
	}
	sort.Slice(entries, func(a, b int) bool {
		if entries[a].OriginIndex != entries[b].OriginIndex {
			return entries[a].OriginIndex < entries[b].OriginIndex
		}
		return entries[a].DestinationIndex < entries[b].DestinationIndex
	})
	return Matrix{Entries: entries, Provider: p.Name()}, nil
}

func (p *StraightLineProvider) estimate(origin, destination Point, mode Mode) (meters, seconds int64) {
	direct := HaversineMeters(origin, destination)
	road := direct * p.DetourFactor
	speed, ok := p.SpeedsKPH[mode]
	if !ok || speed <= 0 {
		speed = p.SpeedsKPH[ModeDriving]
	}
	return int64(math.Round(road)), int64(math.Round(road / (speed * 1000 / 3600)))
}

// HaversineMeters is the great-circle distance between two points.
//
// Exported because location validation uses it to detect implausible jumps
// (document 48), and a second copy of this formula is a second set of rounding
// behaviour.
func HaversineMeters(a, b Point) float64 {
	const earthRadiusMeters = 6371000.0
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLon := (b.Lon - a.Lon) * math.Pi / 180

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusMeters * math.Asin(math.Min(1, math.Sqrt(h)))
}
