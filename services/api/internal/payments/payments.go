// Package payments is the seam between the platform and whoever moves money
// (documents 052, 058, 059).
//
// The owner decided on 2026-09-15 that the platform keeps cash and adds
// digital. This is the half that can be built before a provider exists: which
// methods a customer may choose, and the shape the provider behind each one
// must fit.
//
// One rule runs through the package. **A method is offerable only when
// something can process it.** A customer shown a card button that cannot
// charge a card has been lied to by the product, and they find out at the
// worst possible moment — standing next to a driver at the end of a trip. So
// the enabled list is data, the provider registry is checked against it at
// startup, and a method whose provider is missing is refused rather than
// displayed.
package payments

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Method is how a customer pays.
type Method string

const (
	MethodCash   Method = "CASH"
	MethodCard   Method = "CARD"
	MethodWallet Method = "WALLET"
	MethodBank   Method = "BANK"
)

func (m Method) Valid() bool {
	switch m {
	case MethodCash, MethodCard, MethodWallet, MethodBank:
		return true
	default:
		return false
	}
}

// Digital reports whether a method needs a provider to complete it.
//
// Cash is the exception that shapes the rest: it needs no provider, cannot
// fail asynchronously, and is already settled by the time anyone asks.
func (m Method) Digital() bool { return m.Valid() && m != MethodCash }

// Option is a method as the customer sees it.
type Option struct {
	Method Method `json:"method"`
	Label  string `json:"label"`
	// Provider is deliberately not serialised. Which company processes a
	// payment is not the customer's concern and naming it in an API response
	// is a detail that ends up in a client's switch statement.
	Provider string `json:"-"`
	Order    int    `json:"-"`
}

var (
	// ErrUnavailable reports a method this deployment cannot take.
	ErrUnavailable = errors.New("payments: this payment method is not available")
	// ErrUnknownMethod reports a method that is not one of the four.
	ErrUnknownMethod = errors.New("payments: unknown payment method")
	// ErrNoProvider reports a method enabled in the database with no
	// registered provider behind it — a misconfiguration, caught at startup.
	ErrNoProvider = errors.New("payments: no provider is registered for this method")
)

// Provider is whoever actually moves the money.
//
// Nothing implements this yet, and that is the honest state: there are no
// credentials and no integration. The interface exists so that adding one is a
// type and a line of registration rather than surgery on the settlement path —
// the same shape the notification sender uses, and for the same reason.
//
// Authorize is called when a customer confirms; Capture when the service is
// delivered. Document 052: "authorize at confirmation, capture after
// completion", with immediate-capture providers doing both at checkout.
type Provider interface {
	// Name is the identifier stored on an intent and in a webhook route.
	Name() string
	// Methods lists what this provider can take.
	Methods() []Method
	// Authorize reserves the amount against the customer's instrument and
	// returns the provider's reference plus whatever the app must do next —
	// a redirect, an OTP page, a wallet deep link.
	Authorize(ctx context.Context, req AuthorizeRequest) (Authorization, error)
	// Capture takes an authorized amount.
	Capture(ctx context.Context, providerReference string, amount money.Amount) error
	// VerifyWebhook authenticates a callback before anything acts on it.
	VerifyWebhook(payload []byte, signature string) error
}

// AuthorizeRequest is what a provider needs to reserve an amount.
type AuthorizeRequest struct {
	IntentID       string
	CustomerUserID string
	Amount         money.Amount
	Method         Method
	Description    string
	IdempotencyKey string
}

// Authorization is the provider's answer.
type Authorization struct {
	ProviderReference string
	// Action is what the customer's app must do to complete the payment, when
	// anything is needed. Empty means the authorization is already final.
	Action    *Action
	ExpiresAt *time.Time
}

// Action is a step the customer has to take.
type Action struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
}

// Gateway holds the providers this process has, and answers what a customer
// may choose.
type Gateway struct {
	byMethod map[Method]Provider
}

func NewGateway(providers ...Provider) *Gateway {
	g := &Gateway{byMethod: map[Method]Provider{}}
	for _, provider := range providers {
		for _, method := range provider.Methods() {
			g.byMethod[method] = provider
		}
	}
	return g
}

// ProviderFor returns the provider that handles a method.
func (g *Gateway) ProviderFor(method Method) (Provider, bool) {
	if g == nil {
		return nil, false
	}
	provider, ok := g.byMethod[method]
	return provider, ok
}

// Available filters the configured options down to the ones that will actually
// work in this process.
//
// A method enabled in the database with no provider registered here is dropped
// and reported, not offered. The database says what the business wants; this
// says what the binary can do, and the customer is shown the intersection.
func (g *Gateway) Available(configured []Option) (offered []Option, misconfigured []Method) {
	for _, option := range configured {
		if !option.Method.Digital() {
			offered = append(offered, option)
			continue
		}
		if _, ok := g.ProviderFor(option.Method); !ok {
			misconfigured = append(misconfigured, option.Method)
			continue
		}
		offered = append(offered, option)
	}
	sort.SliceStable(offered, func(i, j int) bool { return offered[i].Order < offered[j].Order })
	return offered, misconfigured
}

// Check refuses a method the customer may not use.
//
// Called on the write path, not just the read path. A client that renders the
// available list correctly is not a guarantee: the list is a hint, and this is
// the rule.
func (g *Gateway) Check(method Method, configured []Option) error {
	if !method.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownMethod, method)
	}
	offered, _ := g.Available(configured)
	for _, option := range offered {
		if option.Method == method {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUnavailable, method)
}
