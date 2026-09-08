package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GoogleProvider routes through the Google Maps Platform.
//
// It implements Provider against two endpoints: Directions for a single route
// and Distance Matrix for an origins × destinations grid (document 96's two
// shapes). Both are billed per call, which document 104 lists as the platform's
// principal map cost driver, so this type does exactly the calls it is asked
// for and no speculative ones.
//
// It never returns a result it cannot stand behind. Where Google cannot answer
// the question that was actually asked — a truck route, an oversized matrix —
// it returns an error so Service falls back to the straight-line estimator and
// the caller sees ConfidenceEstimated. That is the point of document 95's
// warning: "Do not assume a car route is always valid for a truck."
type GoogleProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
	logger  *slog.Logger
}

// Google's documented Distance Matrix ceilings. Exceeding them is a request
// error, not a partial result, so they are checked before the call is made.
const (
	googleMaxMatrixOrigins      = 25
	googleMaxMatrixDestinations = 25
	googleMaxMatrixElements     = 100
)

var (
	// ErrModeUnsupported means the provider has no profile for the requested
	// mode. Returned rather than substituting a different profile: a truck
	// barred from a lane needs a different route, not a car's route relabelled.
	ErrModeUnsupported = errors.New("routing: provider has no profile for this mode")
	// ErrMatrixTooLarge means the request exceeds the provider's per-call
	// ceiling. Splitting it would multiply a billed call without the caller
	// asking, so the request is refused and the estimator answers instead.
	ErrMatrixTooLarge = errors.New("routing: matrix exceeds the provider's per-request limit")
	// ErrNoAPIKey means the provider was constructed without a credential.
	ErrNoAPIKey = errors.New("routing: google provider requires an API key")
)

// GoogleOption configures a GoogleProvider.
type GoogleOption func(*GoogleProvider)

// WithGoogleHTTPClient replaces the default client. Tests use it to point at an
// httptest server; production has no reason to.
func WithGoogleHTTPClient(client *http.Client) GoogleOption {
	return func(p *GoogleProvider) { p.client = client }
}

// WithGoogleBaseURL replaces the API host, for tests.
func WithGoogleBaseURL(baseURL string) GoogleOption {
	return func(p *GoogleProvider) { p.baseURL = strings.TrimSuffix(baseURL, "/") }
}

// WithGoogleLogger attaches a logger. Document 104 asks for request count,
// success rate, latency and fallback rate; with no metrics system in the
// service yet, structured log lines carry them.
func WithGoogleLogger(logger *slog.Logger) GoogleOption {
	return func(p *GoogleProvider) { p.logger = logger }
}

// NewGoogleProvider builds a provider. The key is required: a provider that
// silently makes unauthenticated calls would fail on every request at runtime
// instead of at startup.
func NewGoogleProvider(apiKey string, opts ...GoogleOption) (*GoogleProvider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrNoAPIKey
	}
	p := &GoogleProvider{
		apiKey:  apiKey,
		baseURL: "https://maps.googleapis.com/maps/api",
		// A routing call sits in the path of a customer waiting for a fare.
		// Waiting longer than this for a road distance is worse than falling
		// back to an estimate.
		client: &http.Client{Timeout: 5 * time.Second},
		logger: slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

func (p *GoogleProvider) Name() string    { return "google" }
func (p *GoogleProvider) Version() string { return "1" }

// travelMode maps the platform's routing profile onto Google's.
//
// Truck has no counterpart. Google Directions offers driving, walking,
// bicycling and transit, plus two_wheeler in the regions where it is enabled;
// none of them model axle weight, height or lane bans. Returning driving here
// would produce a route a Mazda cannot legally take, labelled live.
func travelMode(mode Mode) (string, error) {
	switch mode {
	case ModeDriving, "":
		return "driving", nil
	case ModeMotorcycle:
		return "two_wheeler", nil
	case ModeTruck:
		return "", ErrModeUnsupported
	default:
		return "", fmt.Errorf("%w: %s", ErrModeUnsupported, mode)
	}
}

func formatPoint(p Point) string {
	return strconv.FormatFloat(p.Lat, 'f', -1, 64) + "," + strconv.FormatFloat(p.Lon, 'f', -1, 64)
}

func joinPoints(points []Point) string {
	formatted := make([]string, len(points))
	for i, point := range points {
		formatted[i] = formatPoint(point)
	}
	return strings.Join(formatted, "|")
}

// Route asks Google Directions for a single road route.
func (p *GoogleProvider) Route(ctx context.Context, origin, destination Point, opts Options) (Route, error) {
	if !origin.Valid() || !destination.Valid() {
		return Route{}, ErrBadPoint
	}
	mode, err := travelMode(opts.Mode)
	if err != nil {
		return Route{}, err
	}

	query := url.Values{}
	query.Set("origin", formatPoint(origin))
	query.Set("destination", formatPoint(destination))
	query.Set("mode", mode)
	// departure_time is what unlocks duration_in_traffic. A scheduled booking
	// is routed for when it will actually travel, not for now.
	if opts.DepartAt.IsZero() {
		query.Set("departure_time", "now")
	} else {
		query.Set("departure_time", strconv.FormatInt(opts.DepartAt.Unix(), 10))
	}

	var payload googleDirectionsResponse
	if err := p.get(ctx, "directions/json", query, &payload); err != nil {
		return Route{}, err
	}
	if err := googleStatusError("directions", payload.Status, payload.ErrorMessage); err != nil {
		return Route{}, err
	}
	if len(payload.Routes) == 0 || len(payload.Routes[0].Legs) == 0 {
		return Route{}, fmt.Errorf("routing: google directions returned no route")
	}

	first := payload.Routes[0]
	route := Route{
		Geometry:   first.OverviewPolyline.Points,
		Confidence: ConfidenceLive,
	}
	for _, leg := range first.Legs {
		route.DistanceMeters += leg.Distance.Value
		route.DurationSeconds += leg.Duration.Value
		// A leg without a traffic reading contributes its plain duration, so
		// the total stays a total rather than a sum over a subset.
		if leg.DurationInTraffic != nil {
			route.TrafficDurationSeconds += leg.DurationInTraffic.Value
		} else {
			route.TrafficDurationSeconds += leg.Duration.Value
		}
		route.Legs = append(route.Legs, Leg{
			DistanceMeters:  leg.Distance.Value,
			DurationSeconds: leg.Duration.Value,
		})
	}
	// Only claim a traffic model when it actually differs from the free-flow
	// figure; Service treats a non-zero value as the more honest duration.
	if route.TrafficDurationSeconds == route.DurationSeconds {
		route.TrafficDurationSeconds = 0
	}
	return route, nil
}

// Matrix asks Google Distance Matrix for an origins × destinations grid.
func (p *GoogleProvider) Matrix(ctx context.Context, origins, destinations []Point, opts Options) (Matrix, error) {
	if len(origins) == 0 || len(destinations) == 0 {
		return Matrix{}, ErrEmptyMatrix
	}
	for _, point := range append(append([]Point{}, origins...), destinations...) {
		if !point.Valid() {
			return Matrix{}, ErrBadPoint
		}
	}
	if len(origins) > googleMaxMatrixOrigins ||
		len(destinations) > googleMaxMatrixDestinations ||
		len(origins)*len(destinations) > googleMaxMatrixElements {
		return Matrix{}, fmt.Errorf("%w: %d×%d", ErrMatrixTooLarge, len(origins), len(destinations))
	}
	mode, err := travelMode(opts.Mode)
	if err != nil {
		return Matrix{}, err
	}

	query := url.Values{}
	query.Set("origins", joinPoints(origins))
	query.Set("destinations", joinPoints(destinations))
	query.Set("mode", mode)

	var payload googleMatrixResponse
	if err := p.get(ctx, "distancematrix/json", query, &payload); err != nil {
		return Matrix{}, err
	}
	if err := googleStatusError("distancematrix", payload.Status, payload.ErrorMessage); err != nil {
		return Matrix{}, err
	}
	if len(payload.Rows) != len(origins) {
		return Matrix{}, fmt.Errorf("routing: google returned %d rows for %d origins",
			len(payload.Rows), len(origins))
	}

	entries := make([]MatrixEntry, 0, len(origins)*len(destinations))
	for i, row := range payload.Rows {
		if len(row.Elements) != len(destinations) {
			return Matrix{}, fmt.Errorf("routing: google row %d has %d elements for %d destinations",
				i, len(row.Elements), len(destinations))
		}
		for j, element := range row.Elements {
			// A per-cell failure is normal — one unreachable driver must not
			// discard the whole grid. The cell is omitted, and Best simply
			// does not consider it.
			if element.Status != "OK" {
				continue
			}
			entries = append(entries, MatrixEntry{
				OriginIndex:      i,
				DestinationIndex: j,
				DistanceMeters:   element.Distance.Value,
				DurationSeconds:  element.Duration.Value,
				Confidence:       ConfidenceLive,
			})
		}
	}
	if len(entries) == 0 {
		return Matrix{}, fmt.Errorf("routing: google distance matrix had no reachable pair")
	}
	return Matrix{Entries: entries}, nil
}

// get performs the call and decodes into out.
//
// The API key is a query parameter, which makes the request URL a credential.
// It is added here, at the last moment, and no error path below ever includes
// the URL — an error that logs the key turns every failure into a disclosure.
func (p *GoogleProvider) get(ctx context.Context, path string, query url.Values, out any) error {
	query.Set("key", p.apiKey)
	endpoint := p.baseURL + "/" + path + "?" + query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("routing: build %s request: %w", path, err)
	}

	started := time.Now()
	response, err := p.client.Do(request)
	elapsed := time.Since(started)
	if err != nil {
		// url.Error stringifies the URL, and the URL holds the key.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		p.logger.WarnContext(ctx, "google maps request failed",
			"endpoint", path, "duration_ms", elapsed.Milliseconds(), "error", err.Error())
		return fmt.Errorf("routing: google %s: %w", path, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		p.logger.WarnContext(ctx, "google maps returned a non-200",
			"endpoint", path, "status", response.StatusCode, "duration_ms", elapsed.Milliseconds())
		return fmt.Errorf("routing: google %s: http %d", path, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("routing: decode google %s: %w", path, err)
	}
	p.logger.DebugContext(ctx, "google maps request",
		"endpoint", path, "duration_ms", elapsed.Milliseconds())
	return nil
}

// googleStatusError turns Google's in-body status into an error.
//
// Both endpoints answer HTTP 200 for application-level failures, so a caller
// that checks only the status code treats a rejected key as a successful empty
// result. The error message is included because it names the problem — a
// referer restriction, a disabled API — but never the key itself.
func googleStatusError(endpoint, status, message string) error {
	if status == "OK" {
		return nil
	}
	if message != "" {
		return fmt.Errorf("routing: google %s: %s: %s", endpoint, status, message)
	}
	return fmt.Errorf("routing: google %s: %s", endpoint, status)
}

// --- wire format -------------------------------------------------------------
//
// Only the fields the platform maps onto Route and Matrix are declared. Google
// returns a great deal more; decoding it would be storing a shape nobody reads.

type googleValue struct {
	Value int64 `json:"value"`
}

type googleDirectionsResponse struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	Routes       []struct {
		OverviewPolyline struct {
			Points string `json:"points"`
		} `json:"overview_polyline"`
		Legs []struct {
			Distance          googleValue  `json:"distance"`
			Duration          googleValue  `json:"duration"`
			DurationInTraffic *googleValue `json:"duration_in_traffic"`
		} `json:"legs"`
	} `json:"routes"`
}

type googleMatrixResponse struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	Rows         []struct {
		Elements []struct {
			Status   string      `json:"status"`
			Distance googleValue `json:"distance"`
			Duration googleValue `json:"duration"`
		} `json:"elements"`
	} `json:"rows"`
}
