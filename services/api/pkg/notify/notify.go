// Package notify is the platform's messaging boundary (documents 121, 123).
//
// This is CAP-4's Phase 3 increment, and it exists now because authentication
// requires it: document 20 makes phone OTP the initial authentication method
// and document 28 states plainly that the OTP provider must sit behind an
// interface. Without this package there is no way to ship login.
//
// It is deliberately the whole of CAP-4 for now — a Sender interface and two
// development implementations. Templates, preferences, localisation and push
// are Phase 12; delivery observability is Phase 14. Document 123's requirement
// is that "SMS/email providers can be replaced without changing domain logic",
// and that is the only thing this package guarantees today.
package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// Channel is the transport a message travels over.
type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
)

// Message is one transactional message.
//
// It carries a Purpose rather than a template name because no template system
// exists yet (Phase 12). Purpose is what a provider adapter and an audit log
// both need, and it is stable across the templating that will arrive later.
type Message struct {
	Channel Channel
	// To is an E.164 phone number for SMS, an address for email.
	To      string
	Purpose string
	Subject string
	Body    string
}

func (m Message) Validate() error {
	switch m.Channel {
	case ChannelSMS, ChannelEmail:
	default:
		return fmt.Errorf("notify: unknown channel %q", m.Channel)
	}
	if m.To == "" {
		return errors.New("notify: recipient is required")
	}
	if m.Body == "" {
		return errors.New("notify: body is required")
	}
	if m.Purpose == "" {
		return errors.New("notify: purpose is required")
	}
	return nil
}

// Sender delivers a message. Domain code depends on this and nothing narrower,
// which is what makes a provider replaceable.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Service routes a message to the sender registered for its channel, and falls
// back to the next one when a send fails.
//
// Document 123 allows configured fallback for critical workflows. Login is
// exactly that: a user who cannot receive an OTP cannot use the platform at
// all.
type Service struct {
	senders map[Channel][]Sender
	logger  *slog.Logger
}

func NewService(logger *slog.Logger) *Service {
	return &Service{senders: map[Channel][]Sender{}, logger: logger}
}

// Register appends a sender for a channel. Order is priority order: the first
// registered is tried first.
func (s *Service) Register(channel Channel, sender Sender) {
	s.senders[channel] = append(s.senders[channel], sender)
}

var ErrNoProvider = errors.New("notify: no provider registered for channel")

func (s *Service) Send(ctx context.Context, msg Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	senders := s.senders[msg.Channel]
	if len(senders) == 0 {
		return fmt.Errorf("%w: %s", ErrNoProvider, msg.Channel)
	}

	var errs []error
	for i, sender := range senders {
		err := sender.Send(ctx, msg)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
		// The body is never logged: for an OTP it is the credential itself.
		s.logger.Warn("message provider failed",
			slog.String("channel", string(msg.Channel)),
			slog.String("purpose", msg.Purpose),
			slog.Int("provider_index", i),
			slog.String("error", err.Error()))
	}
	return fmt.Errorf("notify: every provider failed for %s: %w", msg.Channel, errors.Join(errs...))
}

// LogSender writes messages to the log instead of sending them. It is the
// development provider: without it, local login would require a real SMS
// account.
//
// It logs the body — which for an OTP is the code — and so must never be
// registered outside development. Wiring guards this in cmd/api.
type LogSender struct{ logger *slog.Logger }

func NewLogSender(logger *slog.Logger) *LogSender { return &LogSender{logger: logger} }

func (l *LogSender) Send(_ context.Context, msg Message) error {
	l.logger.Info("message (development provider — not delivered)",
		slog.String("channel", string(msg.Channel)),
		slog.String("to", msg.To),
		slog.String("purpose", msg.Purpose),
		slog.String("body", msg.Body))
	return nil
}

// MemorySender records messages for tests to assert against.
type MemorySender struct {
	mu   sync.Mutex
	sent []Message
	err  error
}

func NewMemorySender() *MemorySender { return &MemorySender{} }

// FailWith makes every subsequent send fail, for exercising fallback.
func (m *MemorySender) FailWith(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *MemorySender) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, msg)
	return nil
}

func (m *MemorySender) Sent() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.sent...)
}

func (m *MemorySender) Last() (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return Message{}, false
	}
	return m.sent[len(m.sent)-1], true
}
