package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the persistence boundary for identity. Every query the module runs
// lives here; the service holds no SQL.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// --- users -------------------------------------------------------------------

// Unaliased so the same list works in a SELECT and in an INSERT/UPDATE
// RETURNING clause, where there is no table alias to qualify it.
const userColumns = `id, phone, COALESCE(email, ''), COALESCE(name, ''),
	COALESCE(avatar_url, ''), status, created_at, updated_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Phone, &u.Email, &u.Name, &u.AvatarURL, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

// UserByPhone loads a user and their roles by normalised phone number.
func (s *Store) UserByPhone(ctx context.Context, phone string) (User, error) {
	user, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE phone = $1`, phone))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("load user by phone: %w", err)
	}
	return s.withRoles(ctx, user)
}

// UserByID loads a user and their roles.
func (s *Store) UserByID(ctx context.Context, id string) (User, error) {
	user, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("load user: %w", err)
	}
	return s.withRoles(ctx, user)
}

func (s *Store) withRoles(ctx context.Context, user User) (User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT role FROM user_roles WHERE user_id = $1 ORDER BY role`, user.ID)
	if err != nil {
		return User{}, fmt.Errorf("load roles: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role); err != nil {
			return User{}, fmt.Errorf("scan role: %w", err)
		}
		user.Roles = append(user.Roles, role)
	}
	return user, rows.Err()
}

// EnsureUser resolves a phone number to a user, creating one on first sight.
//
// This is where an account is born: document 28's flow is "Resolve/Create
// User", with no separate registration step. A first-time caller who verifies
// an OTP is a customer by default — the role every account starts with,
// because everyone can book. Elevated roles are granted, never self-assigned.
func (s *Store) EnsureUser(ctx context.Context, phone string, initial Role) (user User, created bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, err := scanUser(tx.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE phone = $1`, phone))
	switch {
	case err == nil:
		user = existing
	case errors.Is(err, pgx.ErrNoRows):
		user, err = scanUser(tx.QueryRow(ctx,
			`INSERT INTO users (phone) VALUES ($1) RETURNING `+userColumns, phone))
		if err != nil {
			return User{}, false, fmt.Errorf("create user: %w", err)
		}
		created = true
		if _, err = tx.Exec(ctx,
			`INSERT INTO user_roles (user_id, role) VALUES ($1, $2)`, user.ID, initial); err != nil {
			return User{}, false, fmt.Errorf("grant initial role: %w", err)
		}
	default:
		return User{}, false, fmt.Errorf("resolve user: %w", err)
	}

	rows, err := tx.Query(ctx, `SELECT role FROM user_roles WHERE user_id = $1 ORDER BY role`, user.ID)
	if err != nil {
		return User{}, false, fmt.Errorf("load roles: %w", err)
	}
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role); err != nil {
			rows.Close()
			return User{}, false, err
		}
		user.Roles = append(user.Roles, role)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return User{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, false, fmt.Errorf("commit: %w", err)
	}
	return user, created, nil
}

// GrantRole adds a role. Idempotent: granting a held role is not an error.
func (s *Store) GrantRole(ctx context.Context, userID string, role Role) error {
	if !role.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidRole, role)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, role)
	if err != nil {
		return fmt.Errorf("grant role: %w", err)
	}
	return nil
}

// RevokeRole removes a role.
func (s *Store) RevokeRole(ctx context.Context, userID string, role Role) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM user_roles WHERE user_id = $1 AND role = $2`, userID, role)
	if err != nil {
		return fmt.Errorf("revoke role: %w", err)
	}
	return nil
}

// UpdateProfile changes the fields a user owns. Phone is not among them: it is
// an identity change and document 20 requires stronger verification for one.
func (s *Store) UpdateProfile(ctx context.Context, userID, name, email, avatarURL string) (User, error) {
	user, err := scanUser(s.pool.QueryRow(ctx,
		`UPDATE users SET name = NULLIF($2, ''), email = NULLIF($3, ''),
		        avatar_url = NULLIF($4, ''), updated_at = now()
		 WHERE id = $1
		 RETURNING `+userColumns,
		userID, name, email, avatarURL))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("update profile: %w", err)
	}
	return s.withRoles(ctx, user)
}

// --- devices -----------------------------------------------------------------

// UpsertDevice records a device and refreshes its last-seen time.
func (s *Store) UpsertDevice(ctx context.Context, d Device) (Device, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO devices (user_id, device_identifier, platform, os, app_version)
		 VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''))
		 ON CONFLICT (user_id, device_identifier) DO UPDATE
		   SET last_seen_at = now(),
		       platform    = COALESCE(NULLIF(EXCLUDED.platform, ''), devices.platform),
		       os          = COALESCE(NULLIF(EXCLUDED.os, ''), devices.os),
		       app_version = COALESCE(NULLIF(EXCLUDED.app_version, ''), devices.app_version)
		 RETURNING id, user_id, device_identifier, COALESCE(platform, ''), COALESCE(os, ''),
		           COALESCE(app_version, ''), created_at, last_seen_at`,
		d.UserID, d.Identifier, d.Platform, d.OS, d.AppVersion).
		Scan(&d.ID, &d.UserID, &d.Identifier, &d.Platform, &d.OS, &d.AppVersion, &d.CreatedAt, &d.LastSeenAt)
	if err != nil {
		return Device{}, fmt.Errorf("upsert device: %w", err)
	}
	return d, nil
}

// KnownDevice reports whether this user has used this device before. A false
// answer is a signal, not a verdict (document 116).
func (s *Store) KnownDevice(ctx context.Context, userID, identifier string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM devices WHERE user_id = $1 AND device_identifier = $2)`,
		userID, identifier).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check device: %w", err)
	}
	return exists, nil
}

// --- sessions ----------------------------------------------------------------

const sessionColumns = `id, user_id, COALESCE(device_id::text, ''), created_at,
	last_used_at, expires_at, revoked_at, COALESCE(revoked_reason, '')`

func scanSession(row pgx.Row) (Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.UserID, &s.DeviceID, &s.CreatedAt, &s.LastUsedAt,
		&s.ExpiresAt, &s.RevokedAt, &s.RevokedReason)
	return s, err
}

// CreateSession opens a session against a refresh token hash.
func (s *Store) CreateSession(ctx context.Context, userID, deviceID string, hash []byte, expiresAt time.Time) (Session, error) {
	var device any
	if deviceID != "" {
		device = deviceID
	}
	session, err := scanSession(s.pool.QueryRow(ctx,
		`INSERT INTO sessions (user_id, device_id, refresh_token_hash, expires_at)
		 VALUES ($1, $2, $3, $4) RETURNING `+sessionColumns,
		userID, device, hash, expiresAt))
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

// SessionByRefreshHash finds a session by the hash of the presented token.
func (s *Store) SessionByRefreshHash(ctx context.Context, hash []byte) (Session, error) {
	session, err := scanSession(s.pool.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE refresh_token_hash = $1`, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("load session: %w", err)
	}
	return session, nil
}

// SessionByID loads a session.
func (s *Store) SessionByID(ctx context.Context, id string) (Session, error) {
	session, err := scanSession(s.pool.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("load session: %w", err)
	}
	return session, nil
}

// RotateRefreshToken swaps a session's refresh hash, conditional on the old one
// still being current.
//
// The condition is the whole point. Two concurrent refreshes with the same
// token both read the same session; without the WHERE clause both would
// succeed and issue two live tokens. With it, exactly one updates a row, and
// the loser learns it lost — which is also how a replayed stolen token is
// detected.
func (s *Store) RotateRefreshToken(ctx context.Context, sessionID, userID string, oldHash, newHash []byte, expiresAt time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE sessions
		    SET refresh_token_hash = $3, last_used_at = now(), expires_at = $4
		  WHERE id = $1 AND refresh_token_hash = $2 AND revoked_at IS NULL`,
		sessionID, oldHash, newHash, expiresAt)
	if err != nil {
		return false, fmt.Errorf("rotate refresh token: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}

	// Remember the superseded hash in the same transaction as the rotation, so
	// a token can never be retired without becoming detectable.
	if _, err := tx.Exec(ctx,
		`INSERT INTO refresh_token_history (token_hash, session_id, user_id)
		 VALUES ($1, $2, $3) ON CONFLICT (token_hash) DO NOTHING`,
		oldHash, sessionID, userID); err != nil {
		return false, fmt.Errorf("record rotated token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

// RetiredToken reports the session a superseded refresh token belonged to.
//
// A hit means someone presented a token that was genuinely issued and has
// since been rotated — the definition of refresh-token reuse.
func (s *Store) RetiredToken(ctx context.Context, hash []byte) (sessionID, userID string, found bool, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT session_id::text, user_id::text FROM refresh_token_history WHERE token_hash = $1`,
		hash).Scan(&sessionID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("look up retired token: %w", err)
	}
	return sessionID, userID, true, nil
}

// RevokeSession ends one session.
func (s *Store) RevokeSession(ctx context.Context, sessionID, reason string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now(), revoked_reason = $2
		  WHERE id = $1 AND revoked_at IS NULL`, sessionID, reason)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// RevokeUserSessions ends every live session for a user — "logout all devices"
// (document 116), and the response to a detected token reuse.
func (s *Store) RevokeUserSessions(ctx context.Context, userID, reason string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now(), revoked_reason = $2
		  WHERE user_id = $1 AND revoked_at IS NULL`, userID, reason)
	if err != nil {
		return 0, fmt.Errorf("revoke user sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ActiveSessions lists a user's live sessions, for an account-security screen.
func (s *Store) ActiveSessions(ctx context.Context, userID string) ([]Session, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+sessionColumns+` FROM sessions
		  WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		  ORDER BY last_used_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

// --- OTP challenges ----------------------------------------------------------

// CreateChallenge stores a challenge, superseding any outstanding one for the
// same phone and purpose so two codes are never live at once.
func (s *Store) CreateChallenge(ctx context.Context, c Challenge) (Challenge, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Challenge{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`UPDATE otp_challenges SET consumed_at = now()
		  WHERE phone = $1 AND purpose = $2 AND consumed_at IS NULL`,
		c.Phone, c.Purpose); err != nil {
		return Challenge{}, fmt.Errorf("supersede challenge: %w", err)
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO otp_challenges (phone, purpose, code_hash, max_attempts, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, phone, purpose, code_hash, attempts, max_attempts, expires_at, consumed_at, created_at`,
		c.Phone, c.Purpose, c.CodeHash, c.MaxAttempts, c.ExpiresAt).
		Scan(&c.ID, &c.Phone, &c.Purpose, &c.CodeHash, &c.Attempts, &c.MaxAttempts,
			&c.ExpiresAt, &c.ConsumedAt, &c.CreatedAt)
	if err != nil {
		return Challenge{}, fmt.Errorf("create challenge: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Challenge{}, fmt.Errorf("commit: %w", err)
	}
	return c, nil
}

// LiveChallenge loads the outstanding challenge for a phone and purpose.
func (s *Store) LiveChallenge(ctx context.Context, phone string, purpose OTPPurpose) (Challenge, error) {
	var c Challenge
	err := s.pool.QueryRow(ctx,
		`SELECT id, phone, purpose, code_hash, attempts, max_attempts, expires_at, consumed_at, created_at
		   FROM otp_challenges
		  WHERE phone = $1 AND purpose = $2 AND consumed_at IS NULL`,
		phone, purpose).
		Scan(&c.ID, &c.Phone, &c.Purpose, &c.CodeHash, &c.Attempts, &c.MaxAttempts,
			&c.ExpiresAt, &c.ConsumedAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Challenge{}, ErrChallengeNotFound
	}
	if err != nil {
		return Challenge{}, fmt.Errorf("load challenge: %w", err)
	}
	return c, nil
}

// RecordFailedAttempt increments the attempt counter and reports the new count.
func (s *Store) RecordFailedAttempt(ctx context.Context, challengeID string) (int, error) {
	var attempts int
	err := s.pool.QueryRow(ctx,
		`UPDATE otp_challenges SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts`,
		challengeID).Scan(&attempts)
	if err != nil {
		return 0, fmt.Errorf("record attempt: %w", err)
	}
	return attempts, nil
}

// ConsumeChallenge marks a challenge used, and reports whether this call was
// the one that consumed it.
//
// Conditional for the same reason as refresh rotation: two requests racing with
// a correct code must produce one login, not two. Single-use is a property of
// the OTP (document 123), and this is where it is enforced.
func (s *Store) ConsumeChallenge(ctx context.Context, challengeID string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE otp_challenges SET consumed_at = now()
		  WHERE id = $1 AND consumed_at IS NULL`, challengeID)
	if err != nil {
		return false, fmt.Errorf("consume challenge: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// --- audit -------------------------------------------------------------------

// AppendAudit writes a security event. Failure to audit must not fail the
// operation being audited, so callers log the error and continue; the caller
// decides, not this function.
func (s *Store) AppendAudit(ctx context.Context, a Audit) error {
	metadata := a.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	var userID, sessionID, deviceID, ip any
	if a.UserID != "" {
		userID = a.UserID
	}
	if a.SessionID != "" {
		sessionID = a.SessionID
	}
	if a.DeviceID != "" {
		deviceID = a.DeviceID
	}
	if a.IP != "" {
		ip = a.IP
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO security_events (user_id, session_id, device_id, event, ip, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, sessionID, deviceID, a.Event, ip, encoded)
	if err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}
