// Package notify is the one communication layer documents 121, 122 and 124
// describe.
//
// Document 121 states the principle this package exists to enforce: "Business
// services emit events. They should not directly call Twilio, Firebase, email
// providers or other channel vendors." So nothing outside this package knows
// what a push token is. A booking service says a job's status changed; what
// that becomes, on which devices, and whether the person asked not to be told,
// is decided here.
//
// A notification is a row before it is a provider call. That ordering is what
// makes document 122's reliability states — queued, sent, provider accepted,
// failed — answerable at all, and it means a provider outage delays delivery
// rather than losing it.
package notify

import (
	"errors"
	"time"
)

// Channel is a way of reaching somebody (document 121).
type Channel string

const (
	ChannelPush  Channel = "PUSH"
	ChannelSMS   Channel = "SMS"
	ChannelEmail Channel = "EMAIL"
	ChannelInApp Channel = "IN_APP"
)

// Category is document 122's list.
type Category string

const (
	CategoryRide      Category = "RIDE"
	CategoryDelivery  Category = "DELIVERY"
	CategoryOrder     Category = "ORDER"
	CategoryPayment   Category = "PAYMENT"
	CategorySafety    Category = "SAFETY"
	CategorySupport   Category = "SUPPORT"
	CategoryMarketing Category = "MARKETING"
)

// Required reports whether a category may be switched off.
//
// Document 124: some messages "cannot be disabled when required for security,
// payment, active service, safety, legal/transactional requirements". Safety
// and payment are named outright. RIDE, DELIVERY and ORDER are the active
// service — a customer who muted them would sit waiting for a driver who
// arrived ten minutes ago — so the preference is honoured for everything else
// and refused for these.
//
// Expressed as a function rather than a column so the rule cannot be edited
// into nothing by a row update.
func (c Category) Required() bool {
	switch c {
	case CategorySafety, CategoryPayment, CategoryRide, CategoryDelivery, CategoryOrder:
		return true
	default:
		return false
	}
}

func (c Category) Valid() bool {
	switch c {
	case CategoryRide, CategoryDelivery, CategoryOrder, CategoryPayment,
		CategorySafety, CategorySupport, CategoryMarketing:
		return true
	default:
		return false
	}
}

func (ch Channel) Valid() bool {
	switch ch {
	case ChannelPush, ChannelSMS, ChannelEmail, ChannelInApp:
		return true
	default:
		return false
	}
}

// Platform is where a device runs.
type Platform string

const (
	PlatformIOS     Platform = "IOS"
	PlatformAndroid Platform = "ANDROID"
	PlatformWeb     Platform = "WEB"
)

func (p Platform) Valid() bool {
	return p == PlatformIOS || p == PlatformAndroid || p == PlatformWeb
}

// Device status.
const (
	DeviceActive  = "ACTIVE"
	DeviceRetired = "RETIRED"
	DeviceRevoked = "REVOKED"
)

// Notification status, document 122's reliability list plus SUPPRESSED.
const (
	StatusQueued     = "QUEUED"
	StatusSending    = "SENDING"
	StatusSent       = "SENT"
	StatusDelivered  = "DELIVERED"
	StatusFailed     = "FAILED"
	StatusSuppressed = "SUPPRESSED"
)

// Device is one phone belonging to one account.
type Device struct {
	ID         string    `json:"id"`
	UserID     string    `json:"-"`
	DeviceID   string    `json:"device_id"`
	Platform   Platform  `json:"platform"`
	PushToken  string    `json:"-"`
	AppVersion string    `json:"app_version,omitempty"`
	Status     string    `json:"status"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// Message is what a business service asks for.
//
// It carries no token, no provider and no device: those are this package's
// business, and a caller that could name a device would be a caller that has
// to know which of somebody's phones is awake.
type Message struct {
	UserID   string
	Category Category
	Title    string
	Body     string
	// DeepLink is document 122's stable route into the app.
	DeepLink string
	Data     map[string]any
	// IdempotencyKey stops one event buzzing a phone twice. A retried
	// transition, a replayed webhook and a sweeper running the same pass twice
	// are all ordinary, and a person who feels two buzzes for one arrival
	// learns to ignore the first.
	IdempotencyKey string
}

// Notification is one queued message to one device.
type Notification struct {
	ID        string
	UserID    string
	DeviceID  string
	Channel   Channel
	Category  Category
	Title     string
	Body      string
	DeepLink  string
	Data      map[string]any
	Status    string
	Attempts  int
	PushToken string
	Platform  Platform
	CreatedAt time.Time
}

// MaxAttempts bounds retries before a notification is abandoned.
//
// A push that has failed three times is not going to succeed on the fourth:
// the usual causes are an unregistered token or a malformed payload, and
// neither improves with waiting. Retrying forever turns one bad row into a
// permanent share of every send pass.
const MaxAttempts = 3

var (
	// ErrInvalid reports a registration this package will not store.
	ErrInvalid = errors.New("notify: invalid")
	// ErrNotFound reports a device that is not this user's.
	ErrNotFound = errors.New("notify: not found")
	// ErrRequired reports an attempt to switch off a category document 124
	// does not allow to be switched off.
	ErrRequired = errors.New("notify: this category cannot be disabled")
)
