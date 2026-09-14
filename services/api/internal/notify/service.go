package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Sender is a channel adapter — document 121's bottom layer.
//
// The whole reason this interface exists is that nothing above it may know
// what Firebase is. A booking service emits an event, this package decides who
// and what, and exactly one implementation behind this interface talks to a
// vendor. Swapping FCM for something else is a new type here and a line in
// main.go.
type Sender interface {
	// Send delivers one notification and returns the provider's reference.
	// ErrUnregistered means the token is dead and the device should be
	// retired rather than retried.
	Send(ctx context.Context, n Notification) (providerReference string, err error)
}

// ErrUnregistered reports a push token the provider no longer recognises.
var ErrUnregistered = errors.New("notify: the provider does not recognise this token")

// Service turns a domain event into queued notifications.
type Service struct {
	store  *Store
	logger *slog.Logger
}

func NewService(store *Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, logger: logger}
}

// Notify queues a message to every live device a user has.
//
// Document 122: "A user may have multiple active devices. Do not assume one
// user equals one push token." A person with a phone and a tablet is told on
// both, because the one they are holding is the one that matters and the
// platform cannot know which that is.
//
// A user with no registered device still gets a row, on the IN_APP channel.
// The notification then exists to be read when they next open the app, which
// is the difference between "we could not reach you" and "we never tried".
func (s *Service) Notify(ctx context.Context, msg Message) (int, error) {
	if msg.UserID == "" {
		return 0, fmt.Errorf("%w: a notification needs a recipient", ErrInvalid)
	}
	if !msg.Category.Valid() {
		return 0, fmt.Errorf("%w: unknown category %q", ErrInvalid, msg.Category)
	}

	allowed, err := s.store.Allows(ctx, msg.UserID, ChannelPush, msg.Category)
	if err != nil {
		return 0, err
	}

	devices, err := s.store.DevicesOf(ctx, msg.UserID)
	if err != nil {
		return 0, err
	}

	base := Notification{
		UserID: msg.UserID, Category: msg.Category, Title: msg.Title,
		Body: msg.Body, DeepLink: msg.DeepLink, Data: msg.Data,
	}

	// No device, or the person opted out of push for this category: the
	// message still becomes a row. A suppressed notification is recorded
	// rather than dropped, because "why was I not told?" is a support question
	// and an absent row cannot answer it.
	if len(devices) == 0 || !allowed {
		inApp := base
		inApp.Channel = ChannelInApp
		if !allowed {
			inApp.Status = StatusSuppressed
		}
		if _, err := s.store.Enqueue(ctx, inApp, msg.IdempotencyKey); err != nil {
			return 0, err
		}
		return 0, nil
	}

	queued := 0
	for _, device := range devices {
		push := base
		push.Channel = ChannelPush
		push.DeviceID = device.ID
		// The key is per device, not per message: one event reaching two
		// phones is one notification each, and both must survive a retry of
		// the event that produced them.
		key := ""
		if msg.IdempotencyKey != "" {
			key = msg.IdempotencyKey + ":" + device.ID
		}
		if _, err := s.store.Enqueue(ctx, push, key); err != nil {
			return queued, err
		}
		queued++
	}
	return queued, nil
}

// --- the send pass -----------------------------------------------------------

// Worker drains the queue through a Sender.
//
// Separate from the enqueue path on purpose. A provider outage must delay
// delivery rather than fail the booking that caused it, and that is only true
// if the thing calling the provider is not on the request's goroutine.
type Worker struct {
	store    *Store
	sender   Sender
	logger   *slog.Logger
	interval time.Duration
	batch    int
}

func NewWorker(store *Store, sender Sender, logger *slog.Logger, interval time.Duration) *Worker {
	if interval <= 0 {
		// A notification that arrives five seconds after the driver did is
		// not a notification. This is the visible latency floor for anything
		// a person is waiting on.
		interval = 2 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{store: store, sender: sender, logger: logger, interval: interval, batch: 100}
}

// Run drains until the context is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("notification worker stopped")
			return
		case <-ticker.C:
			if sent, err := w.Pass(ctx); err != nil {
				w.logger.Error("a notification pass failed", slog.String("error", err.Error()))
			} else if sent > 0 {
				w.logger.Debug("notifications sent", slog.Int("count", sent))
			}
		}
	}
}

// Pass sends one batch. Exported so a test can drive it without a clock.
func (w *Worker) Pass(ctx context.Context) (int, error) {
	claimed, err := w.store.Claim(ctx, w.batch)
	if err != nil {
		return 0, err
	}

	sent := 0
	for _, n := range claimed {
		// A push with no token is a device revoked between queueing and
		// sending. Failing it with a reason is more useful than handing the
		// provider an empty string and reading its complaint back.
		if n.Channel == ChannelPush && n.PushToken == "" {
			_ = w.store.MarkFailed(ctx, n.ID, "no active token for this device", MaxAttempts)
			continue
		}
		// IN_APP has no provider: the row is the delivery. It is marked sent
		// so the queue does not carry it forever.
		if n.Channel == ChannelInApp {
			if err := w.store.MarkSent(ctx, n.ID, ""); err != nil {
				return sent, err
			}
			sent++
			continue
		}

		reference, err := w.sender.Send(ctx, n)
		switch {
		case err == nil:
			if err := w.store.MarkSent(ctx, n.ID, reference); err != nil {
				return sent, err
			}
			sent++
		case errors.Is(err, ErrUnregistered):
			// The token is dead, not the notification. Retiring the device
			// stops every future send to it; retrying this one would fail
			// twice more for the same reason.
			if n.DeviceID != "" {
				_ = w.store.RetireDevice(ctx, n.DeviceID)
			}
			_ = w.store.MarkFailed(ctx, n.ID, "the provider does not recognise this token", MaxAttempts)
		default:
			_ = w.store.MarkFailed(ctx, n.ID, err.Error(), n.Attempts)
		}
	}
	return sent, nil
}

// --- the only sender that ships ----------------------------------------------

// LogSender records what would have been sent.
//
// This platform has no push credentials and no provider configured, and an
// adapter that pretended otherwise would be the worst of both: a booking that
// believes a customer was told, and a customer who was not. So the shipping
// adapter writes the notification to the log and reports success, the queue
// drains, the reliability states are exercised end to end, and the one thing
// that does not happen is a phone buzzing.
//
// Replacing this with FCM is a new type implementing Sender and one line in
// main.go. Nothing above this interface changes.
type LogSender struct{ logger *slog.Logger }

func NewLogSender(logger *slog.Logger) LogSender {
	if logger == nil {
		logger = slog.Default()
	}
	return LogSender{logger: logger}
}

func (l LogSender) Send(_ context.Context, n Notification) (string, error) {
	l.logger.Info("notification not delivered: no push provider is configured",
		slog.String("notification_id", n.ID),
		slog.String("user_id", n.UserID),
		slog.String("category", string(n.Category)),
		slog.String("platform", string(n.Platform)),
		slog.String("title", n.Title))
	return "", nil
}
