package identity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/authn"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/notify"
	"github.com/sarmadkung/rideme/services/api/pkg/phone"
	"github.com/sarmadkung/rideme/services/api/pkg/ratelimit"
)

// Service implements the authentication flow document 28 specifies:
//
//	Phone -> Request OTP -> Verify OTP -> Resolve/Create User
//	      -> Issue Access Token -> Issue Refresh Token -> Load User + Roles
type Service struct {
	store    *Store
	issuer   *authn.Issuer
	sender   notify.Sender
	limiter  ratelimit.Limiter
	logger   *slog.Logger
	secret   string
	region   phone.Region
	now      func() time.Time
	otpTTL   time.Duration
	attempts int
}

// Options configures the service. Every duration and count here is an
// engineering default, not a documented value: document 28 requires OTPs to
// "expire quickly" and verification to be attempt-limited without stating
// either number.
type Options struct {
	OTPTTL      time.Duration
	MaxAttempts int
	Region      phone.Region
	Now         func() time.Time
}

const (
	// DefaultOTPTTL is long enough for an SMS to arrive on a slow network and
	// be typed, short enough that an intercepted code is stale quickly.
	DefaultOTPTTL = 5 * time.Minute
	// DefaultMaxAttempts bounds guessing: five tries against a six-digit code
	// is a 1-in-200,000 chance, and a real user who mistypes twice is not
	// locked out.
	DefaultMaxAttempts = 5
)

// Rate limits. Documents 20 and 28 require limiting by phone, IP and device
// and state no numbers; these are starting values, tunable from operations.
var (
	RuleOTPPerPhone  = ratelimit.Rule{Name: "otp_request_phone", Limit: 5, Window: time.Hour}
	RuleOTPPerIP     = ratelimit.Rule{Name: "otp_request_ip", Limit: 20, Window: time.Hour}
	RuleVerifyPerIP  = ratelimit.Rule{Name: "otp_verify_ip", Limit: 30, Window: time.Hour}
	RuleRefreshPerIP = ratelimit.Rule{Name: "refresh_ip", Limit: 120, Window: time.Hour}
)

func NewService(
	store *Store,
	issuer *authn.Issuer,
	sender notify.Sender,
	limiter ratelimit.Limiter,
	logger *slog.Logger,
	secret string,
	opts Options,
) *Service {
	if opts.OTPTTL == 0 {
		opts.OTPTTL = DefaultOTPTTL
	}
	if opts.MaxAttempts == 0 {
		opts.MaxAttempts = DefaultMaxAttempts
	}
	if opts.Region.CallingCode == "" {
		opts.Region = phone.Pakistan
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{
		store: store, issuer: issuer, sender: sender, limiter: limiter,
		logger: logger, secret: secret, region: opts.Region, now: opts.Now,
		otpTTL: opts.OTPTTL, attempts: opts.MaxAttempts,
	}
}

// RequestContext carries the request-scoped signals authentication decisions
// and audits need (document 116).
type RequestContext struct {
	IP         string
	DeviceID   string
	Platform   string
	OS         string
	AppVersion string
}

// --- OTP request -------------------------------------------------------------

// RequestOTP issues a code for a phone number.
//
// It returns the same result whether or not an account exists. Document 28
// requires preventing account enumeration, and the only way to do that is for
// the response to carry no information about the account — including its
// timing and its error shape.
func (s *Service) RequestOTP(ctx context.Context, rawPhone string, purpose OTPPurpose, rc RequestContext) (expiresAt time.Time, err error) {
	normalized, err := phone.Normalize(rawPhone, s.region)
	if err != nil {
		return time.Time{}, httpx.Validation("phone number is not valid",
			map[string]string{"phone": err.Error()})
	}

	if err := s.checkLimit(ctx, RuleOTPPerPhone, normalized, rc, ""); err != nil {
		return time.Time{}, err
	}
	if rc.IP != "" {
		if err := s.checkLimit(ctx, RuleOTPPerIP, rc.IP, rc, ""); err != nil {
			return time.Time{}, err
		}
	}

	code, err := authn.NewOTP()
	if err != nil {
		return time.Time{}, httpx.Internal("could not issue a code").WithCause(err)
	}

	expiresAt = s.now().UTC().Add(s.otpTTL)
	if _, err := s.store.CreateChallenge(ctx, Challenge{
		Phone:       normalized,
		Purpose:     purpose,
		CodeHash:    authn.HashOTP(s.secret, normalized, code),
		MaxAttempts: s.attempts,
		ExpiresAt:   expiresAt,
	}); err != nil {
		return time.Time{}, httpx.Internal("could not issue a code").WithCause(err)
	}

	if err := s.sender.Send(ctx, notify.Message{
		Channel: notify.ChannelSMS,
		To:      normalized,
		Purpose: "auth.otp",
		Body:    fmt.Sprintf("%s is your RideMe verification code. It expires in %d minutes.", code, int(s.otpTTL.Minutes())),
	}); err != nil {
		// The challenge is already stored. Reporting a failure here is honest:
		// the user will never receive this code and should be told to retry
		// rather than made to wait for an SMS that is not coming.
		return time.Time{}, httpx.Unavailable("could not deliver the code, please try again").WithCause(err)
	}

	s.audit(ctx, Audit{
		Event:    EventOTPRequested,
		IP:       rc.IP,
		Metadata: map[string]any{"phone": phone.Mask(normalized), "purpose": string(purpose)},
	})
	return expiresAt, nil
}

// --- OTP verification --------------------------------------------------------

// Tokens is a freshly issued credential pair.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Session      Session
	User         User
}

// VerifyOTP checks a code and, on success, resolves or creates the user and
// opens a session.
func (s *Service) VerifyOTP(ctx context.Context, rawPhone, code string, purpose OTPPurpose, rc RequestContext) (Tokens, error) {
	normalized, err := phone.Normalize(rawPhone, s.region)
	if err != nil {
		return Tokens{}, httpx.Validation("phone number is not valid",
			map[string]string{"phone": err.Error()})
	}
	if rc.IP != "" {
		if err := s.checkLimit(ctx, RuleVerifyPerIP, rc.IP, rc, ""); err != nil {
			return Tokens{}, err
		}
	}

	// One error for every failure mode below. A caller must not be able to
	// tell "no such challenge" from "wrong code" — the first answers whether
	// an account exists.
	invalid := httpx.Unauthorized("the code is incorrect or has expired")

	challenge, err := s.store.LiveChallenge(ctx, normalized, purpose)
	if errors.Is(err, ErrChallengeNotFound) {
		return Tokens{}, invalid
	}
	if err != nil {
		return Tokens{}, httpx.Internal("could not verify the code").WithCause(err)
	}
	if !challenge.Usable(s.now().UTC()) {
		return Tokens{}, invalid
	}

	if !authn.EqualOTP(challenge.CodeHash, authn.HashOTP(s.secret, normalized, code)) {
		attempts, aerr := s.store.RecordFailedAttempt(ctx, challenge.ID)
		if aerr != nil {
			s.logger.Error("could not record a failed OTP attempt", slog.String("error", aerr.Error()))
		}
		s.audit(ctx, Audit{
			Event:    EventOTPFailed,
			IP:       rc.IP,
			Metadata: map[string]any{"phone": phone.Mask(normalized), "attempts": attempts},
		})
		return Tokens{}, invalid
	}

	// Single-use, enforced by the database. If another request consumed this
	// challenge first, this one did not authenticate anybody.
	consumed, err := s.store.ConsumeChallenge(ctx, challenge.ID)
	if err != nil {
		return Tokens{}, httpx.Internal("could not verify the code").WithCause(err)
	}
	if !consumed {
		return Tokens{}, invalid
	}

	user, created, err := s.store.EnsureUser(ctx, normalized, RoleCustomer)
	if err != nil {
		return Tokens{}, httpx.Internal("could not resolve the account").WithCause(err)
	}
	if !user.CanAuthenticate() {
		s.audit(ctx, Audit{UserID: user.ID, Event: EventAccountSuspended, IP: rc.IP})
		return Tokens{}, httpx.Forbidden("this account is not active")
	}

	tokens, err := s.openSession(ctx, user, rc)
	if err != nil {
		return Tokens{}, err
	}
	s.audit(ctx, Audit{
		UserID:    user.ID,
		SessionID: tokens.Session.ID,
		DeviceID:  tokens.Session.DeviceID,
		Event:     EventLogin,
		IP:        rc.IP,
		Metadata:  map[string]any{"new_account": created, "purpose": string(purpose)},
	})
	return tokens, nil
}

// openSession records the device, opens a session and mints both tokens.
func (s *Service) openSession(ctx context.Context, user User, rc RequestContext) (Tokens, error) {
	var deviceID string
	if rc.DeviceID != "" {
		known, err := s.store.KnownDevice(ctx, user.ID, rc.DeviceID)
		if err != nil {
			s.logger.Warn("could not check device history", slog.String("error", err.Error()))
		}
		device, err := s.store.UpsertDevice(ctx, Device{
			UserID:     user.ID,
			Identifier: rc.DeviceID,
			Platform:   rc.Platform,
			OS:         rc.OS,
			AppVersion: rc.AppVersion,
		})
		if err != nil {
			s.logger.Warn("could not record device", slog.String("error", err.Error()))
		} else {
			deviceID = device.ID
		}
		// A first sighting is a signal for review, not grounds to refuse
		// (document 116: "without breaking normal usage"). Every legitimate
		// new phone starts here.
		if !known && deviceID != "" {
			s.audit(ctx, Audit{
				UserID: user.ID, DeviceID: deviceID,
				Event: EventSuspiciousDevice, IP: rc.IP,
				Metadata: map[string]any{"reason": "first sighting of this device"},
			})
		}
	}

	refreshToken, refreshHash, err := authn.NewRefreshToken()
	if err != nil {
		return Tokens{}, httpx.Internal("could not open a session").WithCause(err)
	}
	session, err := s.store.CreateSession(ctx, user.ID, deviceID, refreshHash,
		s.now().UTC().Add(authn.RefreshTokenTTL))
	if err != nil {
		return Tokens{}, httpx.Internal("could not open a session").WithCause(err)
	}

	accessToken, claims, err := s.issuer.Issue(user.ID, session.ID, user.RoleStrings())
	if err != nil {
		return Tokens{}, httpx.Internal("could not issue a token").WithCause(err)
	}

	return Tokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Unix(claims.ExpiresAt, 0).UTC(),
		Session:      session,
		User:         user,
	}, nil
}

// --- refresh -----------------------------------------------------------------

// Refresh exchanges a refresh token for a new pair, rotating the refresh token
// (document 28).
//
// Rotation makes theft detectable. A stolen token works exactly once, and when
// the real client next refreshes with the token it still holds, that token no
// longer matches — which is the reuse this method treats as a compromise.
func (s *Service) Refresh(ctx context.Context, refreshToken string, rc RequestContext) (Tokens, error) {
	if rc.IP != "" {
		if err := s.checkLimit(ctx, RuleRefreshPerIP, rc.IP, rc, ""); err != nil {
			return Tokens{}, err
		}
	}
	invalid := httpx.Unauthorized("the session is no longer valid")

	hash := authn.HashRefreshToken(refreshToken)
	session, err := s.store.SessionByRefreshHash(ctx, hash)
	if errors.Is(err, ErrSessionNotFound) {
		// The token is not current. It may be random, or it may be a token
		// this platform issued and has since rotated away — which is theft,
		// and looks identical unless the retired hashes are checked.
		s.detectReuse(ctx, hash, rc)
		return Tokens{}, invalid
	}
	if err != nil {
		return Tokens{}, httpx.Internal("could not refresh the session").WithCause(err)
	}

	if !session.Active(s.now().UTC()) {
		// A revoked session presented with a valid-looking token is either a
		// stale client or a stolen credential. Both warrant a record.
		s.audit(ctx, Audit{
			UserID: session.UserID, SessionID: session.ID,
			Event: EventRefreshReuse, IP: rc.IP,
			Metadata: map[string]any{"reason": "refresh against an inactive session"},
		})
		return Tokens{}, invalid
	}

	newToken, newHash, err := authn.NewRefreshToken()
	if err != nil {
		return Tokens{}, httpx.Internal("could not refresh the session").WithCause(err)
	}
	rotated, err := s.store.RotateRefreshToken(ctx, session.ID, session.UserID, hash, newHash,
		s.now().UTC().Add(authn.RefreshTokenTTL))
	if err != nil {
		return Tokens{}, httpx.Internal("could not refresh the session").WithCause(err)
	}
	if !rotated {
		// The token was valid a moment ago and is not now: something else
		// rotated it. Either two clients hold the same token, or one of them
		// should not. Ending every session is the safe reading, and forces a
		// fresh login the legitimate user can complete.
		revoked, rerr := s.store.RevokeUserSessions(ctx, session.UserID, "refresh token reuse detected")
		if rerr != nil {
			s.logger.Error("could not revoke sessions after reuse", slog.String("error", rerr.Error()))
		}
		s.audit(ctx, Audit{
			UserID: session.UserID, SessionID: session.ID,
			Event: EventRefreshReuse, IP: rc.IP,
			Metadata: map[string]any{"sessions_revoked": revoked},
		})
		return Tokens{}, invalid
	}

	user, err := s.store.UserByID(ctx, session.UserID)
	if err != nil {
		return Tokens{}, httpx.Internal("could not load the account").WithCause(err)
	}
	if !user.CanAuthenticate() {
		_ = s.store.RevokeSession(ctx, session.ID, "account is not active")
		return Tokens{}, httpx.Forbidden("this account is not active")
	}

	// Roles are re-read here rather than carried forward, so a revoked role
	// stops working at the next refresh rather than at the next login.
	accessToken, claims, err := s.issuer.Issue(user.ID, session.ID, user.RoleStrings())
	if err != nil {
		return Tokens{}, httpx.Internal("could not issue a token").WithCause(err)
	}
	s.audit(ctx, Audit{UserID: user.ID, SessionID: session.ID, Event: EventRefreshed, IP: rc.IP})

	return Tokens{
		AccessToken:  accessToken,
		RefreshToken: newToken,
		ExpiresAt:    time.Unix(claims.ExpiresAt, 0).UTC(),
		Session:      session,
		User:         user,
	}, nil
}

// detectReuse checks whether an unrecognised token is one the platform
// previously issued, and if so treats it as a compromise.
//
// Revoking every session is deliberately blunt. The alternative — revoking
// only the session the token belonged to — leaves an attacker who also holds a
// current token still logged in. The cost is that the legitimate user logs in
// again, which they can do; the cost of the alternative is that the attacker
// stays.
func (s *Service) detectReuse(ctx context.Context, hash []byte, rc RequestContext) {
	sessionID, userID, found, err := s.store.RetiredToken(ctx, hash)
	if err != nil {
		s.logger.Error("could not check retired refresh tokens", slog.String("error", err.Error()))
		return
	}
	if !found {
		return
	}
	revoked, err := s.store.RevokeUserSessions(ctx, userID, "refresh token reuse detected")
	if err != nil {
		s.logger.Error("could not revoke sessions after reuse", slog.String("error", err.Error()))
	}
	s.audit(ctx, Audit{
		UserID: userID, SessionID: sessionID,
		Event: EventRefreshReuse, IP: rc.IP,
		Metadata: map[string]any{"sessions_revoked": revoked, "reason": "a rotated refresh token was presented again"},
	})
}

// --- logout ------------------------------------------------------------------

// Logout ends one session.
func (s *Service) Logout(ctx context.Context, sessionID, userID string, rc RequestContext) error {
	if err := s.store.RevokeSession(ctx, sessionID, "logout"); err != nil {
		return httpx.Internal("could not end the session").WithCause(err)
	}
	s.audit(ctx, Audit{UserID: userID, SessionID: sessionID, Event: EventLogout, IP: rc.IP})
	return nil
}

// LogoutAll ends every session for a user (document 116).
func (s *Service) LogoutAll(ctx context.Context, userID string, rc RequestContext) (int64, error) {
	count, err := s.store.RevokeUserSessions(ctx, userID, "logout all devices")
	if err != nil {
		return 0, httpx.Internal("could not end the sessions").WithCause(err)
	}
	s.audit(ctx, Audit{UserID: userID, Event: EventLogoutAll, IP: rc.IP,
		Metadata: map[string]any{"sessions_revoked": count}})
	return count, nil
}

// --- profile -----------------------------------------------------------------

// Me loads the authenticated user.
func (s *Service) Me(ctx context.Context, userID string) (User, error) {
	user, err := s.store.UserByID(ctx, userID)
	if errors.Is(err, ErrUserNotFound) {
		return User{}, httpx.NotFound("account not found")
	}
	if err != nil {
		return User{}, httpx.Internal("could not load the account").WithCause(err)
	}
	return user, nil
}

// UpdateProfile changes the fields a user owns.
func (s *Service) UpdateProfile(ctx context.Context, userID, name, email, avatarURL string) (User, error) {
	user, err := s.store.UpdateProfile(ctx, userID, name, email, avatarURL)
	if errors.Is(err, ErrUserNotFound) {
		return User{}, httpx.NotFound("account not found")
	}
	if err != nil {
		return User{}, httpx.Internal("could not update the profile").WithCause(err)
	}
	return user, nil
}

// Sessions lists a user's live sessions.
func (s *Service) Sessions(ctx context.Context, userID string) ([]Session, error) {
	sessions, err := s.store.ActiveSessions(ctx, userID)
	if err != nil {
		return nil, httpx.Internal("could not list sessions").WithCause(err)
	}
	return sessions, nil
}

// GrantRole elevates an account. Callers must already have checked that the
// actor may do this; the check is in the handler's authorization, not here.
func (s *Service) GrantRole(ctx context.Context, actorID, userID string, role Role, rc RequestContext) error {
	if !role.Valid() {
		return httpx.Validation("unknown role", map[string]string{"role": string(role)})
	}
	if err := s.store.GrantRole(ctx, userID, role); err != nil {
		return httpx.Internal("could not grant the role").WithCause(err)
	}
	s.audit(ctx, Audit{
		UserID: userID, Event: EventRoleChanged, IP: rc.IP,
		Metadata: map[string]any{"granted": string(role), "actor_id": actorID},
	})
	return nil
}

// RevokeRole removes a role.
func (s *Service) RevokeRole(ctx context.Context, actorID, userID string, role Role, rc RequestContext) error {
	if err := s.store.RevokeRole(ctx, userID, role); err != nil {
		return httpx.Internal("could not revoke the role").WithCause(err)
	}
	s.audit(ctx, Audit{
		UserID: userID, Event: EventRoleChanged, IP: rc.IP,
		Metadata: map[string]any{"revoked": string(role), "actor_id": actorID},
	})
	return nil
}

// --- helpers -----------------------------------------------------------------

func (s *Service) checkLimit(ctx context.Context, rule ratelimit.Rule, key string, rc RequestContext, userID string) error {
	decision, err := s.limiter.Allow(ctx, rule, key)
	if err != nil {
		// A limiter that cannot be reached must not take authentication down
		// with it. The failure is logged and the request proceeds — the
		// alternative is that a Redis outage locks every user out.
		s.logger.Error("rate limiter unavailable, allowing request",
			slog.String("rule", rule.Name), slog.String("error", err.Error()))
		return nil
	}
	if !decision.Allowed {
		s.audit(ctx, Audit{
			UserID: userID, Event: EventRateLimited, IP: rc.IP,
			Metadata: map[string]any{"rule": rule.Name},
		})
		return httpx.RateLimited("too many attempts, please try again later")
	}
	return nil
}

// audit records a security event, never failing the operation it describes.
func (s *Service) audit(ctx context.Context, a Audit) {
	// The request context may already be cancelled when this runs after a
	// failure; the audit still needs to be written.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.store.AppendAudit(writeCtx, a); err != nil {
		s.logger.Error("could not append security event",
			slog.String("event", string(a.Event)), slog.String("error", err.Error()))
	}
}
