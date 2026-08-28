// Package realtime is the WebSocket gateway (documents 18, 47, 103).
//
// Document 47's objective is the constraint that shapes everything here:
// "Deliver live job and driver state without turning WebSocket connections
// into the system of record." A client that misses an event must be able to
// recover by asking for state, not by replaying history — so this package
// delivers, drops and coalesces freely, and never stores.
package realtime

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// EventType is the set document 18 lists.
type EventType string

const (
	EventJobAssigned      EventType = "job.assigned"
	EventJobAccepted      EventType = "job.accepted"
	EventJobStatusChanged EventType = "job.status_changed"
	EventDriverLocation   EventType = "driver.location"
	EventDriverOnline     EventType = "driver.online"
	EventDriverOffline    EventType = "driver.offline"
	EventQuoteUpdated     EventType = "quote.updated"
	EventPaymentUpdated   EventType = "payment.updated"
	EventSupportUpdated   EventType = "support.updated"
)

// Envelope is document 47's event shape, exactly.
//
// Version is on every event from the first one. Adding it later means every
// consumer must handle its absence, and the events that would most need
// versioning — payment, assignment — are the ones already in flight by then.
type Envelope struct {
	EventID    string    `json:"event_id"`
	Type       EventType `json:"type"`
	Version    int       `json:"version"`
	OccurredAt time.Time `json:"occurred_at"`
	ResourceID string    `json:"resource_id"`
	Payload    any       `json:"payload"`
}

// ChannelKind is the addressable scope of a subscription.
type ChannelKind string

const (
	ChannelUser     ChannelKind = "user"
	ChannelDriver   ChannelKind = "driver"
	ChannelJob      ChannelKind = "job"
	ChannelMerchant ChannelKind = "merchant"
	ChannelAdminOps ChannelKind = "admin"
)

// Channel is a subscription target — document 47's user:{id}, driver:{id},
// job:{id}, merchant:{id}, admin:operations.
type Channel struct {
	Kind ChannelKind
	ID   string
}

func (c Channel) String() string {
	if c.Kind == ChannelAdminOps {
		return "admin:operations"
	}
	return string(c.Kind) + ":" + c.ID
}

var ErrBadChannel = errors.New("realtime: malformed channel")

// ParseChannel reads a channel name from a client.
//
// It is strict because the client controls this string. Document 47 forbids
// arbitrary channel subscription, and the first line of that defence is
// refusing to construct a channel the platform does not define.
func ParseChannel(raw string) (Channel, error) {
	if raw == "admin:operations" {
		return Channel{Kind: ChannelAdminOps}, nil
	}
	kind, id, found := strings.Cut(raw, ":")
	if !found || id == "" || strings.Contains(id, ":") {
		return Channel{}, fmt.Errorf("%w: %q", ErrBadChannel, raw)
	}
	switch ChannelKind(kind) {
	case ChannelUser, ChannelDriver, ChannelJob, ChannelMerchant:
		return Channel{Kind: ChannelKind(kind), ID: id}, nil
	default:
		return Channel{}, fmt.Errorf("%w: unknown kind %q", ErrBadChannel, kind)
	}
}

// Subscriber identifies who is asking, so authorization has something to check.
type Subscriber struct {
	UserID   string
	DriverID string
	Roles    []string
}

func (s Subscriber) hasRole(role string) bool {
	for _, held := range s.Roles {
		if held == role {
			return true
		}
	}
	return false
}

// Authorizer decides whether a subscriber may join a channel. Job and merchant
// membership need a database, so the hub takes this as an interface rather
// than importing a store.
type Authorizer interface {
	CanSubscribe(sub Subscriber, ch Channel) (bool, error)
}

// RoleAuthorizer answers the checks that need no database, and delegates the
// rest.
//
// Document 47: "Authorization is mandatory before subscription." The default
// here is refusal — an unknown channel kind or a missing delegate denies,
// because a gateway that fails open leaks every driver's position.
type RoleAuthorizer struct {
	// Membership answers job and merchant scoping. Nil denies both.
	Membership func(sub Subscriber, ch Channel) (bool, error)
}

func (a RoleAuthorizer) CanSubscribe(sub Subscriber, ch Channel) (bool, error) {
	switch ch.Kind {
	case ChannelUser:
		// Your own channel and nobody else's.
		return sub.UserID != "" && sub.UserID == ch.ID, nil
	case ChannelDriver:
		if sub.DriverID != "" && sub.DriverID == ch.ID {
			return true, nil
		}
		// Operations may watch a driver; document 102 requires that access be
		// audited, which the tracking store does when the position is read.
		return sub.hasRole("ADMIN") || sub.hasRole("SUPER_ADMIN") || sub.hasRole("SUPPORT"), nil
	case ChannelAdminOps:
		return sub.hasRole("ADMIN") || sub.hasRole("SUPER_ADMIN") || sub.hasRole("SUPPORT"), nil
	case ChannelJob, ChannelMerchant:
		if a.Membership == nil {
			return false, nil
		}
		return a.Membership(sub, ch)
	default:
		return false, nil
	}
}

// Connection is one client.
type Connection struct {
	ID         string
	Subscriber Subscriber

	mu       sync.Mutex
	channels map[string]bool
	out      chan Envelope
	closed   bool
	// dropped counts events discarded because this client could not keep up.
	dropped int
	// coalesced holds the latest pending location per resource, so a slow
	// client receives the driver's current position rather than a backlog of
	// where they used to be.
	coalesced map[string]Envelope
}

// bufferSize bounds what one client can hold. Document 47 requires bounded
// buffers so slow clients cannot destabilize the backend; the number is a
// trade between tolerating a brief stall and holding memory per connection.
const bufferSize = 64

func newConnection(id string, sub Subscriber) *Connection {
	return &Connection{
		ID: id, Subscriber: sub,
		channels:  map[string]bool{},
		out:       make(chan Envelope, bufferSize),
		coalesced: map[string]Envelope{},
	}
}

// Events is the stream a transport writes to the socket.
func (c *Connection) Events() <-chan Envelope { return c.out }

// Dropped reports how many events this connection lost to backpressure.
func (c *Connection) Dropped() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped
}

// Channels lists current subscriptions.
func (c *Connection) Channels() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.channels))
	for name := range c.channels {
		out = append(out, name)
	}
	return out
}

func (c *Connection) send(event Envelope) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	select {
	case c.out <- event:
	default:
		// The buffer is full. Document 47 requires coalescing high-frequency
		// location rather than queueing it: the newest position replaces the
		// pending one, because a stale position is not worth delivering.
		c.mu.Lock()
		if event.Type == EventDriverLocation {
			c.coalesced[event.ResourceID] = event
		} else {
			c.dropped++
		}
		c.mu.Unlock()
	}
}

// Flush delivers coalesced location events once the client has caught up.
func (c *Connection) Flush() {
	c.mu.Lock()
	pending := c.coalesced
	c.coalesced = map[string]Envelope{}
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return
	}
	for _, event := range pending {
		select {
		case c.out <- event:
		default:
			c.mu.Lock()
			c.coalesced[event.ResourceID] = event
			c.mu.Unlock()
			return
		}
	}
}

// Hub routes events to subscribed connections.
type Hub struct {
	mu            sync.RWMutex
	connections   map[string]*Connection
	byChannel     map[string]map[string]*Connection
	authorizer    Authorizer
	maxPerUser    int
	perUserCounts map[string]int
}

// MaxConnectionsPerUser bounds how many sockets one account may hold.
// Document 47 lists connection limits under Security: without one, a single
// account can exhaust the gateway.
const MaxConnectionsPerUser = 10

func NewHub(authorizer Authorizer) *Hub {
	return &Hub{
		connections:   map[string]*Connection{},
		byChannel:     map[string]map[string]*Connection{},
		authorizer:    authorizer,
		maxPerUser:    MaxConnectionsPerUser,
		perUserCounts: map[string]int{},
	}
}

var (
	ErrNotAuthorized  = errors.New("realtime: not authorized for this channel")
	ErrTooManyClients = errors.New("realtime: connection limit reached")
	ErrClosed         = errors.New("realtime: connection is closed")
)

// Connect registers a client.
func (h *Hub) Connect(id string, sub Subscriber) (*Connection, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if sub.UserID != "" && h.perUserCounts[sub.UserID] >= h.maxPerUser {
		return nil, ErrTooManyClients
	}
	conn := newConnection(id, sub)
	h.connections[id] = conn
	if sub.UserID != "" {
		h.perUserCounts[sub.UserID]++
	}
	return conn, nil
}

// Subscribe joins a connection to a channel after authorizing it.
func (h *Hub) Subscribe(conn *Connection, ch Channel) error {
	allowed, err := h.authorizer.CanSubscribe(conn.Subscriber, ch)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrNotAuthorized
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	conn.mu.Lock()
	if conn.closed {
		conn.mu.Unlock()
		return ErrClosed
	}
	conn.channels[ch.String()] = true
	conn.mu.Unlock()

	name := ch.String()
	if h.byChannel[name] == nil {
		h.byChannel[name] = map[string]*Connection{}
	}
	h.byChannel[name][conn.ID] = conn
	return nil
}

// Unsubscribe leaves a channel.
func (h *Hub) Unsubscribe(conn *Connection, ch Channel) {
	h.mu.Lock()
	defer h.mu.Unlock()

	name := ch.String()
	conn.mu.Lock()
	delete(conn.channels, name)
	conn.mu.Unlock()
	if subs := h.byChannel[name]; subs != nil {
		delete(subs, conn.ID)
		if len(subs) == 0 {
			delete(h.byChannel, name)
		}
	}
}

// Disconnect removes a client and every subscription it held.
func (h *Hub) Disconnect(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn.mu.Lock()
	if conn.closed {
		conn.mu.Unlock()
		return
	}
	conn.closed = true
	channels := make([]string, 0, len(conn.channels))
	for name := range conn.channels {
		channels = append(channels, name)
	}
	conn.channels = map[string]bool{}
	close(conn.out)
	conn.mu.Unlock()

	for _, name := range channels {
		if subs := h.byChannel[name]; subs != nil {
			delete(subs, conn.ID)
			if len(subs) == 0 {
				delete(h.byChannel, name)
			}
		}
	}
	delete(h.connections, conn.ID)
	if conn.Subscriber.UserID != "" {
		h.perUserCounts[conn.Subscriber.UserID]--
		if h.perUserCounts[conn.Subscriber.UserID] <= 0 {
			delete(h.perUserCounts, conn.Subscriber.UserID)
		}
	}
}

// Publish delivers an event to every subscriber of a channel.
//
// Delivery is best-effort by design: this is a notification path, not the
// system of record. A client that misses an event recovers by fetching state
// on reconnect (document 47), which is why dropping under backpressure is
// correct rather than merely tolerable.
func (h *Hub) Publish(ch Channel, event Envelope) int {
	h.mu.RLock()
	subs := h.byChannel[ch.String()]
	targets := make([]*Connection, 0, len(subs))
	for _, conn := range subs {
		targets = append(targets, conn)
	}
	h.mu.RUnlock()

	if event.Version == 0 {
		event.Version = 1
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	for _, conn := range targets {
		conn.send(event)
	}
	return len(targets)
}

// SubscriberCount reports how many connections hold a channel.
func (h *Hub) SubscriberCount(ch Channel) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.byChannel[ch.String()])
}

// ConnectionCount reports total live connections.
func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}
