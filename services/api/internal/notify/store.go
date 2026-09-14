package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const deviceColumns = `id::text, user_id::text, device_id, platform, push_token,
	COALESCE(app_version,''), status, last_seen_at`

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.UserID, &d.DeviceID, &d.Platform, &d.PushToken,
		&d.AppVersion, &d.Status, &d.LastSeenAt)
	return d, err
}

// RegisterDevice records or refreshes a phone's push token.
//
// Two conflicts are possible and they mean different things. The same device
// re-registering is an app restart, and its row is updated. The same *token*
// arriving under a different account is a phone that changed hands — the
// provider reissues tokens lazily, so the old row must be revoked rather than
// left delivering somebody's ride updates to whoever owns the handset now.
func (s *Store) RegisterDevice(ctx context.Context, d Device) (Device, error) {
	if d.UserID == "" || strings.TrimSpace(d.DeviceID) == "" || strings.TrimSpace(d.PushToken) == "" {
		return Device{}, fmt.Errorf("%w: a device needs an owner, an id and a token", ErrInvalid)
	}
	if !d.Platform.Valid() {
		return Device{}, fmt.Errorf("%w: unknown platform %q", ErrInvalid, d.Platform)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Device{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`UPDATE device_tokens SET status = 'REVOKED', updated_at = now()
		  WHERE push_token = $1 AND status = 'ACTIVE'
		    AND NOT (user_id = $2 AND device_id = $3)`,
		d.PushToken, d.UserID, d.DeviceID); err != nil {
		return Device{}, fmt.Errorf("revoke a reassigned token: %w", err)
	}

	var appVersion any
	if d.AppVersion != "" {
		appVersion = d.AppVersion
	}
	registered, err := scanDevice(tx.QueryRow(ctx,
		`INSERT INTO device_tokens (user_id, device_id, platform, push_token, app_version)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (user_id, device_id) DO UPDATE
		   SET platform = EXCLUDED.platform, push_token = EXCLUDED.push_token,
		       app_version = EXCLUDED.app_version, status = 'ACTIVE',
		       last_seen_at = now(), updated_at = now()
		 RETURNING `+deviceColumns,
		d.UserID, d.DeviceID, d.Platform, d.PushToken, appVersion))
	if err != nil {
		return Device{}, fmt.Errorf("register device: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Device{}, fmt.Errorf("commit: %w", err)
	}
	return registered, nil
}

// RevokeDevice stops delivery to one of a user's devices — a sign-out.
func (s *Store) RevokeDevice(ctx context.Context, userID, deviceID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE device_tokens SET status = 'REVOKED', updated_at = now()
		  WHERE user_id = $1 AND device_id = $2 AND status <> 'REVOKED'`, userID, deviceID)
	if err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RetireDevice marks a token the provider rejected as unregistered.
//
// Distinct from revoking: nobody signed out, the app was uninstalled or the
// token rotated. Keeping the two apart is what lets "this person turned it
// off" be told from "this phone is gone" when someone asks why a driver
// stopped receiving offers.
func (s *Store) RetireDevice(ctx context.Context, deviceRowID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE device_tokens SET status = 'RETIRED', updated_at = now() WHERE id = $1`, deviceRowID)
	if err != nil {
		return fmt.Errorf("retire device: %w", err)
	}
	return nil
}

// DevicesOf lists a user's live devices.
func (s *Store) DevicesOf(ctx context.Context, userID string) ([]Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+deviceColumns+` FROM device_tokens
		  WHERE user_id = $1 AND status = 'ACTIVE' ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	devices := make([]Device, 0, 4)
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// --- preferences (document 124) ----------------------------------------------

// Allows reports whether a user wants this channel and category.
//
// Absence means yes. A user who has never opened the settings screen has no
// rows, and an account that had to be seeded with a dozen defaults before it
// could be told anything would be an account whose first notification depends
// on a migration having run.
func (s *Store) Allows(ctx context.Context, userID string, channel Channel, category Category) (bool, error) {
	if category.Required() {
		return true, nil
	}
	var enabled bool
	err := s.pool.QueryRow(ctx,
		`SELECT enabled FROM notification_preferences
		  WHERE user_id = $1 AND channel = $2 AND category = $3`,
		userID, channel, category).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read preference: %w", err)
	}
	return enabled, nil
}

// SetPreference records an opt-out or an opt-in.
//
// A required category refuses rather than silently storing a setting nothing
// honours. A switch that appears to work and does nothing is worse than one
// that says it cannot be turned off.
func (s *Store) SetPreference(ctx context.Context, userID string, channel Channel, category Category, enabled bool) error {
	if !channel.Valid() || !category.Valid() {
		return fmt.Errorf("%w: unknown channel or category", ErrInvalid)
	}
	if !enabled && category.Required() {
		return fmt.Errorf("%w: %s", ErrRequired, category)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO notification_preferences (user_id, channel, category, enabled)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (user_id, channel, category) DO UPDATE
		   SET enabled = EXCLUDED.enabled, updated_at = now()`,
		userID, channel, category, enabled)
	if err != nil {
		return fmt.Errorf("set preference: %w", err)
	}
	return nil
}

// Preference is one stored setting.
type Preference struct {
	Channel  Channel  `json:"channel"`
	Category Category `json:"category"`
	Enabled  bool     `json:"enabled"`
}

// PreferencesOf lists what a user has changed from the default.
func (s *Store) PreferencesOf(ctx context.Context, userID string) ([]Preference, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT channel, category, enabled FROM notification_preferences
		  WHERE user_id = $1 ORDER BY channel, category`, userID)
	if err != nil {
		return nil, fmt.Errorf("list preferences: %w", err)
	}
	defer rows.Close()

	out := make([]Preference, 0, 8)
	for rows.Next() {
		var p Preference
		if err := rows.Scan(&p.Channel, &p.Category, &p.Enabled); err != nil {
			return nil, fmt.Errorf("scan preference: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- the queue ---------------------------------------------------------------

// Enqueue stores one notification for one device.
//
// A repeated idempotency key returns the existing row rather than a second
// one. One domain event must produce one buzz however many times the thing
// that produced it is retried.
func (s *Store) Enqueue(ctx context.Context, n Notification, idempotencyKey string) (string, error) {
	payload, err := json.Marshal(n.Data)
	if err != nil {
		return "", fmt.Errorf("encode notification data: %w", err)
	}
	if n.Data == nil {
		payload = []byte(`{}`)
	}
	var deviceID, deepLink, key any
	if n.DeviceID != "" {
		deviceID = n.DeviceID
	}
	if n.DeepLink != "" {
		deepLink = n.DeepLink
	}
	if idempotencyKey != "" {
		key = idempotencyKey
	}
	status := n.Status
	if status == "" {
		status = StatusQueued
	}

	var id string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO notifications (user_id, device_id, channel, category, title, body,
		                            deep_link, data, status, idempotency_key)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (idempotency_key) DO UPDATE SET user_id = notifications.user_id
		 RETURNING id::text`,
		n.UserID, deviceID, n.Channel, n.Category, n.Title, n.Body,
		deepLink, payload, status, key).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("enqueue notification: %w", err)
	}
	return id, nil
}

// Claim takes a batch of pending notifications and marks them SENDING.
//
// SKIP LOCKED so two workers never take the same row: document 185 requires
// that duplicated messages produce no duplicate effects, and two processes
// racing on the same queue is the ordinary way a person gets told twice.
func (s *Store) Claim(ctx context.Context, limit int) ([]Notification, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		UPDATE notifications SET status = 'SENDING', attempts = attempts + 1
		 WHERE id IN (
		     SELECT n.id FROM notifications n
		      WHERE n.status = 'QUEUED' AND n.attempts < $2
		      ORDER BY n.created_at
		      LIMIT $1
		      FOR UPDATE SKIP LOCKED)
		 RETURNING id::text, user_id::text, COALESCE(device_id::text,''), channel, category,
		           title, body, COALESCE(deep_link,''), data, status, attempts`,
		limit, MaxAttempts)
	if err != nil {
		return nil, fmt.Errorf("claim notifications: %w", err)
	}
	defer rows.Close()

	claimed := make([]Notification, 0, limit)
	for rows.Next() {
		var n Notification
		var data []byte
		if err := rows.Scan(&n.ID, &n.UserID, &n.DeviceID, &n.Channel, &n.Category,
			&n.Title, &n.Body, &n.DeepLink, &data, &n.Status, &n.Attempts); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		if len(data) > 0 {
			_ = json.Unmarshal(data, &n.Data)
		}
		claimed = append(claimed, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// The token is read after the claim rather than joined into it: the update
	// above must lock notifications and nothing else, and a device revoked
	// between queueing and sending should be discovered here.
	for i := range claimed {
		if claimed[i].DeviceID == "" {
			continue
		}
		var token string
		var platform Platform
		var status string
		err := s.pool.QueryRow(ctx,
			`SELECT push_token, platform, status FROM device_tokens WHERE id = $1`,
			claimed[i].DeviceID).Scan(&token, &platform, &status)
		if err != nil || status != DeviceActive {
			continue
		}
		claimed[i].PushToken, claimed[i].Platform = token, platform
	}
	return claimed, nil
}

// MarkSent records a provider accepting a notification.
func (s *Store) MarkSent(ctx context.Context, id, providerReference string) error {
	var ref any
	if providerReference != "" {
		ref = providerReference
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE notifications SET status = 'SENT', sent_at = now(), provider_reference = $2
		  WHERE id = $1`, id, ref)
	if err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	return nil
}

// MarkFailed records a failure, returning the row to the queue while attempts
// remain.
//
// A notification that has exhausted its attempts stays FAILED with its reason.
// Retrying forever turns one unregistered token into a permanent share of
// every send pass, and the reason is what tells an operator which.
func (s *Store) MarkFailed(ctx context.Context, id, reason string, attempts int) error {
	status := StatusQueued
	if attempts >= MaxAttempts {
		status = StatusFailed
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE notifications SET status = $2, failure_reason = $3 WHERE id = $1`,
		id, status, reason)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

// PendingCount reports the queue depth, for the readiness surface and for a
// test to assert on.
func (s *Store) PendingCount(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE status IN ('QUEUED','SENDING')`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending notifications: %w", err)
	}
	return count, nil
}

// Recent lists a user's notifications, newest first — the in-app inbox.
func (s *Store) Recent(ctx context.Context, userID string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, channel, category, title, body, COALESCE(deep_link,''), status, created_at
		   FROM notifications WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	out := make([]Notification, 0, limit)
	for rows.Next() {
		var n Notification
		var created time.Time
		if err := rows.Scan(&n.ID, &n.Channel, &n.Category, &n.Title, &n.Body,
			&n.DeepLink, &n.Status, &created); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		n.UserID, n.CreatedAt = userID, created
		out = append(out, n)
	}
	return out, rows.Err()
}
