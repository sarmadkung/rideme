// Package pricing is CAP-1's boundary: the platform's single fare engine.
//
// It is service-parameterized from the first line, and that is deliberate.
// Document 05 gives four fare formulas — ride, parcel, cargo, grocery — which
// all share `base + distance`:
//
//	ride    base + distance + time + demand + vehicle adjustment
//	parcel  base + distance + size/weight + urgency
//	cargo   base + distance + vehicle + capacity + loading + waiting + schedule
//
// Four formulas, one shape. Implemented four times they drift four ways, and
// the drift shows up as a customer charged differently for the same kilometre
// depending on which module priced it. Here a service is a Tariff row and a
// set of Components; adding parcel in Phase 9 is a rule set, not an engine.
//
// **No rate, fee or fare value appears in this package.** Document 34: "Rates
// are configuration, not hard-coded business logic." Every number comes from a
// Tariff loaded from the database.
package pricing

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

// Component is one named line of a quote breakdown (document 34).
type Component string

const (
	ComponentBase         Component = "base"
	ComponentDistance     Component = "distance"
	ComponentTime         Component = "time"
	ComponentWaiting      Component = "waiting"
	ComponentLoading      Component = "loading"
	ComponentWeight       Component = "weight"
	ComponentServiceFee   Component = "service_fee"
	ComponentDemand       Component = "demand"
	ComponentDiscount     Component = "discount"
	ComponentTax          Component = "tax"
	ComponentMinimumTopUp Component = "minimum_fare_top_up"
)

// Tariff is the configuration a quote is computed from. Every field is an
// integer: minor units for amounts, basis points for rates. There is no float
// anywhere in a monetary path (ADR-008).
type Tariff struct {
	ID          string
	JobType     string
	VehicleType string
	City        string
	Version     int
	Currency    money.Currency

	MinimumFareMinor      int64
	BaseMinor             int64
	PerKMMinor            int64
	PerMinuteMinor        int64
	WaitingPerMinuteMinor int64
	LoadingPerMinuteMinor int64
	PerKGMinor            int64
	ServiceFeeMinor       int64
	ServiceFeeBPS         int
	TaxBPS                int

	DemandMinBPS int
	DemandMaxBPS int
}

// Request is what a quote is computed for.
type Request struct {
	JobType     string
	VehicleType string
	City        string

	DistanceMeters  int64
	DurationSeconds int64
	// WaitingSeconds and LoadingSeconds are billable components for cargo
	// (document 05). They are recorded as events regardless; whether they are
	// priced is BD-13, and a tariff with zero rates prices them at nothing.
	WaitingSeconds int64
	LoadingSeconds int64
	WeightKG       float64

	// DemandBPS is the demand multiplier in basis points, 10000 meaning 1.0.
	// BD-02 is unresolved: nothing computes this today, so it arrives as zero
	// and is normalised to 10000 — the term present and inert, exactly as the
	// register recommends.
	DemandBPS int

	DiscountMinor   int64
	RouteConfidence routing.Confidence
	RequestedBy     string
}

// Line is one component of the breakdown.
type Line struct {
	Component Component    `json:"component"`
	Amount    money.Amount `json:"amount"`
	// Detail explains how the line was derived, so a disputed fare can be
	// reconstructed without rerunning the engine.
	Detail string `json:"detail,omitempty"`
}

// Quote is a priced offer. It carries the full breakdown document 34 requires,
// plus the tariff version that produced it so the price can be reproduced.
type Quote struct {
	JobType         string             `json:"job_type"`
	VehicleType     string             `json:"vehicle_type"`
	Currency        money.Currency     `json:"currency"`
	Lines           []Line             `json:"lines"`
	Total           money.Amount       `json:"total"`
	TariffID        string             `json:"tariff_id"`
	PricingVersion  int                `json:"pricing_version"`
	DistanceMeters  int64              `json:"distance_meters"`
	DurationSeconds int64              `json:"duration_seconds"`
	RouteConfidence routing.Confidence `json:"route_confidence"`
	ExpiresAt       time.Time          `json:"expires_at"`
}

// Component returns one line of the breakdown.
func (q Quote) Component(name Component) (money.Amount, bool) {
	for _, line := range q.Lines {
		if line.Component == name {
			return line.Amount, true
		}
	}
	return money.Amount{}, false
}

// QuoteTTL is how long a quote stands.
//
// Document 34: "Quotes expire because supply, demand and route estimates can
// change." No period is stated; five minutes is long enough to read a fare and
// confirm, short enough that a route estimate is still roughly true. An
// engineering default, recorded as one.
const QuoteTTL = 5 * time.Minute

var (
	ErrNoTariff        = errors.New("pricing: no tariff configured for this service")
	ErrUnknownService  = errors.New("pricing: unknown job type")
	ErrDemandUnbounded = errors.New("pricing: demand adjustment outside the configured bounds")
)

// Engine computes quotes. It holds no rates — only a clock.
type Engine struct {
	now func() time.Time
}

func NewEngine(now func() time.Time) *Engine {
	if now == nil {
		now = time.Now
	}
	return &Engine{now: now}
}

// Quote prices a request against a tariff.
//
// The order is fixed and matters: components accumulate into a subtotal, the
// minimum fare tops it up, demand scales it, and tax applies last. Applying
// tax before demand would tax a figure the customer never sees; applying the
// minimum after demand would let a bounded multiplier push a fare below the
// floor it is supposed to have.
func (e *Engine) Quote(req Request, tariff Tariff) (Quote, error) {
	currency := tariff.Currency
	if currency == "" {
		currency = money.PKR
	}
	zero, err := money.Zero(currency)
	if err != nil {
		return Quote{}, err
	}

	rules, ok := ruleSets[req.JobType]
	if !ok {
		return Quote{}, fmt.Errorf("%w: %q", ErrUnknownService, req.JobType)
	}

	quote := Quote{
		JobType:         req.JobType,
		VehicleType:     req.VehicleType,
		Currency:        currency,
		TariffID:        tariff.ID,
		PricingVersion:  tariff.Version,
		DistanceMeters:  req.DistanceMeters,
		DurationSeconds: req.DurationSeconds,
		RouteConfidence: req.RouteConfidence,
		ExpiresAt:       e.now().UTC().Add(QuoteTTL),
	}

	subtotal := zero
	for _, rule := range rules {
		line, err := rule(req, tariff, currency)
		if err != nil {
			return Quote{}, err
		}
		if line.Amount.IsZero() && line.Component != ComponentBase {
			// A zero line is noise in a breakdown a customer reads. Base is
			// kept even at zero so the shape of the fare is always visible.
			continue
		}
		if subtotal, err = subtotal.Add(line.Amount); err != nil {
			return Quote{}, err
		}
		quote.Lines = append(quote.Lines, line)
	}

	// Minimum fare, before demand and tax.
	if tariff.MinimumFareMinor > 0 && subtotal.Minor < tariff.MinimumFareMinor {
		topUp, err := money.New(tariff.MinimumFareMinor-subtotal.Minor, currency)
		if err != nil {
			return Quote{}, err
		}
		quote.Lines = append(quote.Lines, Line{
			Component: ComponentMinimumTopUp, Amount: topUp,
			Detail: fmt.Sprintf("minimum fare %d minor units", tariff.MinimumFareMinor),
		})
		if subtotal, err = subtotal.Add(topUp); err != nil {
			return Quote{}, err
		}
	}

	// Demand. Bounded by the tariff, which document 34 requires: "Do not
	// introduce uncontrolled surge."
	demandBPS := req.DemandBPS
	if demandBPS == 0 {
		demandBPS = 10000 // 1.0 — the term present and inert (BD-02)
	}
	minBPS, maxBPS := tariff.DemandMinBPS, tariff.DemandMaxBPS
	if minBPS == 0 && maxBPS == 0 {
		minBPS, maxBPS = 10000, 10000
	}
	if demandBPS < minBPS || demandBPS > maxBPS {
		return Quote{}, fmt.Errorf("%w: %d not in [%d, %d]", ErrDemandUnbounded, demandBPS, minBPS, maxBPS)
	}
	if demandBPS != 10000 {
		scaled, err := subtotal.ApplyRate(int64(demandBPS), 10000)
		if err != nil {
			return Quote{}, err
		}
		adjustment, err := scaled.Sub(subtotal)
		if err != nil {
			return Quote{}, err
		}
		quote.Lines = append(quote.Lines, Line{
			Component: ComponentDemand, Amount: adjustment,
			Detail: fmt.Sprintf("%d bps", demandBPS),
		})
		subtotal = scaled
	}

	// Discount, before tax.
	if req.DiscountMinor > 0 {
		discount, err := money.New(req.DiscountMinor, currency)
		if err != nil {
			return Quote{}, err
		}
		// A discount never makes a fare negative; the platform would owe the
		// customer money for taking a ride.
		if discount.Minor > subtotal.Minor {
			discount = subtotal
		}
		negated, err := discount.Neg()
		if err != nil {
			return Quote{}, err
		}
		quote.Lines = append(quote.Lines, Line{Component: ComponentDiscount, Amount: negated})
		if subtotal, err = subtotal.Add(negated); err != nil {
			return Quote{}, err
		}
	}

	// Tax last, on what the customer actually pays.
	if tariff.TaxBPS > 0 {
		tax, err := subtotal.ApplyRate(int64(tariff.TaxBPS), 10000)
		if err != nil {
			return Quote{}, err
		}
		quote.Lines = append(quote.Lines, Line{
			Component: ComponentTax, Amount: tax,
			Detail: fmt.Sprintf("%d bps", tariff.TaxBPS),
		})
		if subtotal, err = subtotal.Add(tax); err != nil {
			return Quote{}, err
		}
	}

	quote.Total = subtotal
	return quote, nil
}

// --- rule sets ---------------------------------------------------------------

// rule computes one component for one service.
type rule func(Request, Tariff, money.Currency) (Line, error)

// ruleSets is the service-specific half of the engine. This map is where a new
// service is added — Phase 9's parcel and cargo, Phase 10's grocery — and
// nothing outside it changes.
//
// Each entry is the component list document 05 gives for that service:
//
//	ride    base + distance + time + demand + vehicle adjustment
//	parcel  base + distance + size/weight + urgency
//	cargo   base + distance + vehicle + capacity + loading + waiting + schedule
//
// GROCERY is absent until Phase 10 builds it — an unpriced service is refused
// rather than quietly charged with another service's rules.
var ruleSets = map[string][]rule{
	"RIDE":   {baseRule, distanceRule, timeRule, serviceFeeRule},
	"PARCEL": {baseRule, distanceRule, WeightRule, serviceFeeRule},
	// Cargo prices loading and waiting time. BD-13 leaves the rates open, so a
	// tariff with zero rates records the time and charges nothing for it.
	"CARGO":   {baseRule, distanceRule, WeightRule, LoadingRule, WaitingRule, serviceFeeRule},
	"FREIGHT": {baseRule, distanceRule, WeightRule, LoadingRule, WaitingRule, serviceFeeRule},
}

// Register adds a service's rule set. Phase 9 and Phase 10 call this rather
// than writing a second engine.
func Register(jobType string, rules ...rule) { ruleSets[jobType] = rules }

// RegisteredServices lists the services the engine can price.
func RegisteredServices() []string {
	out := make([]string, 0, len(ruleSets))
	for jobType := range ruleSets {
		out = append(out, jobType)
	}
	return out
}

func baseRule(_ Request, t Tariff, c money.Currency) (Line, error) {
	amount, err := money.New(t.BaseMinor, c)
	return Line{Component: ComponentBase, Amount: amount}, err
}

func distanceRule(req Request, t Tariff, c money.Currency) (Line, error) {
	if t.PerKMMinor == 0 || req.DistanceMeters <= 0 {
		zero, err := money.Zero(c)
		return Line{Component: ComponentDistance, Amount: zero}, err
	}
	rate, err := money.New(t.PerKMMinor, c)
	if err != nil {
		return Line{}, err
	}
	// Rational arithmetic, not a float multiply: metres/1000 as a rate keeps
	// the whole computation in integers and rounds exactly once.
	amount, err := rate.ApplyRate(req.DistanceMeters, 1000)
	if err != nil {
		return Line{}, err
	}
	return Line{
		Component: ComponentDistance, Amount: amount,
		Detail: fmt.Sprintf("%.2f km", float64(req.DistanceMeters)/1000),
	}, nil
}

func timeRule(req Request, t Tariff, c money.Currency) (Line, error) {
	if t.PerMinuteMinor == 0 || req.DurationSeconds <= 0 {
		zero, err := money.Zero(c)
		return Line{Component: ComponentTime, Amount: zero}, err
	}
	rate, err := money.New(t.PerMinuteMinor, c)
	if err != nil {
		return Line{}, err
	}
	amount, err := rate.ApplyRate(req.DurationSeconds, 60)
	if err != nil {
		return Line{}, err
	}
	return Line{
		Component: ComponentTime, Amount: amount,
		Detail: fmt.Sprintf("%d min", int(math.Round(float64(req.DurationSeconds)/60))),
	}, nil
}

func serviceFeeRule(_ Request, t Tariff, c money.Currency) (Line, error) {
	amount, err := money.New(t.ServiceFeeMinor, c)
	return Line{Component: ComponentServiceFee, Amount: amount}, err
}

// Exported rules for the service slices that follow. They are here rather than
// in those packages so every component stays behind CAP-1's boundary.
var (
	BaseRule       = baseRule
	DistanceRule   = distanceRule
	TimeRule       = timeRule
	ServiceFeeRule = serviceFeeRule
)

// WaitingRule prices billable waiting time (document 05, cargo).
func WaitingRule(req Request, t Tariff, c money.Currency) (Line, error) {
	return perMinute(ComponentWaiting, req.WaitingSeconds, t.WaitingPerMinuteMinor, c)
}

// LoadingRule prices billable loading time (document 05, cargo).
//
// BD-13 leaves the rates unresolved. A tariff with a zero rate records the
// time and prices it at nothing, which is what the register asks for: "Build
// the event recording. The list and the rates are the owner's."
func LoadingRule(req Request, t Tariff, c money.Currency) (Line, error) {
	return perMinute(ComponentLoading, req.LoadingSeconds, t.LoadingPerMinuteMinor, c)
}

// WeightRule prices by weight (document 05, parcel size/weight).
func WeightRule(req Request, t Tariff, c money.Currency) (Line, error) {
	if t.PerKGMinor == 0 || req.WeightKG <= 0 {
		zero, err := money.Zero(c)
		return Line{Component: ComponentWeight, Amount: zero}, err
	}
	rate, err := money.New(t.PerKGMinor, c)
	if err != nil {
		return Line{}, err
	}
	// Grams keep the rational exact rather than rounding the weight first.
	amount, err := rate.ApplyRate(int64(math.Round(req.WeightKG*1000)), 1000)
	if err != nil {
		return Line{}, err
	}
	return Line{Component: ComponentWeight, Amount: amount,
		Detail: fmt.Sprintf("%.1f kg", req.WeightKG)}, nil
}

func perMinute(component Component, seconds, ratePerMinute int64, c money.Currency) (Line, error) {
	if ratePerMinute == 0 || seconds <= 0 {
		zero, err := money.Zero(c)
		return Line{Component: component, Amount: zero}, err
	}
	rate, err := money.New(ratePerMinute, c)
	if err != nil {
		return Line{}, err
	}
	amount, err := rate.ApplyRate(seconds, 60)
	if err != nil {
		return Line{}, err
	}
	return Line{Component: component, Amount: amount,
		Detail: fmt.Sprintf("%d min", int(math.Round(float64(seconds)/60)))}, nil
}
