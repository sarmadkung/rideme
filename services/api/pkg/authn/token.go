// Package authn issues and verifies the platform's credentials (document 28):
// short-lived access tokens, rotating refresh tokens, and OTP codes.
//
// The tokens are signed with HMAC-SHA256 using the process secret. The format
// is a compact JSON payload rather than a JWT because the platform is both
// issuer and only audience: nothing outside this service verifies these
// tokens, so JWT's interoperability buys nothing and its algorithm agility is
// a liability — "alg: none" and algorithm-confusion attacks exist because a
// verifier trusted the token to say how it should be checked. Here the
// algorithm is not negotiable, because it is not in the token.
//
// If a third party ever needs to verify a token, this is the decision to
// revisit.
package authn

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

var (
	ErrMalformed = errors.New("authn: token is malformed")
	ErrSignature = errors.New("authn: token signature is invalid")
	ErrExpired   = errors.New("authn: token has expired")
)

// Claims is the access token payload. Document 28 says "only minimal claims",
// which is a privacy and a staleness argument at once: anything carried here
// is readable by whoever holds the token, and stays true only until it is not.
type Claims struct {
	Subject   string   `json:"sub"`
	SessionID string   `json:"sid"`
	Roles     []string `json:"roles,omitempty"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
}

// Issuer mints and verifies access tokens.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// AccessTokenTTL is short because the access token is not revocable — it is
// verified by signature alone, without a database round trip. Its lifetime is
// therefore the window in which a revoked session still works, and five
// minutes is a deliberate trade of that window against a refresh call per five
// minutes of use. Document 28 requires "short-lived" and states no number.
const AccessTokenTTL = 5 * time.Minute

func NewIssuer(secret string, opts ...IssuerOption) (*Issuer, error) {
	if len(secret) < 32 {
		return nil, errors.New("authn: signing secret must be at least 32 bytes")
	}
	i := &Issuer{secret: []byte(secret), ttl: AccessTokenTTL, now: time.Now}
	for _, opt := range opts {
		opt(i)
	}
	return i, nil
}

type IssuerOption func(*Issuer)

// WithClock is used by tests; production uses time.Now.
func WithClock(now func() time.Time) IssuerOption {
	return func(i *Issuer) { i.now = now }
}

func WithTTL(ttl time.Duration) IssuerOption {
	return func(i *Issuer) { i.ttl = ttl }
}

// Issue mints an access token for a session.
func (i *Issuer) Issue(subject, sessionID string, roles []string) (string, Claims, error) {
	if subject == "" || sessionID == "" {
		return "", Claims{}, errors.New("authn: subject and session are required")
	}
	now := i.now().UTC()
	claims := Claims{
		Subject:   subject,
		SessionID: sessionID,
		Roles:     roles,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(i.ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", Claims{}, err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + i.sign(body), claims, nil
}

// Verify checks the signature before it looks at anything else, so an attacker
// cannot use claim parsing as an oracle.
func (i *Issuer) Verify(token string) (Claims, error) {
	body, signature, found := strings.Cut(token, ".")
	if !found || body == "" || signature == "" {
		return Claims{}, ErrMalformed
	}
	// Constant time: a timing difference here leaks how much of a forged
	// signature was correct.
	if subtle.ConstantTimeCompare([]byte(signature), []byte(i.sign(body))) != 1 {
		return Claims{}, ErrSignature
	}

	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrMalformed
	}
	if claims.Subject == "" || claims.SessionID == "" {
		return Claims{}, ErrMalformed
	}
	if i.now().UTC().Unix() >= claims.ExpiresAt {
		return Claims{}, ErrExpired
	}
	return claims, nil
}

func (i *Issuer) sign(body string) string {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// --- refresh tokens ----------------------------------------------------------

// RefreshTokenTTL is the session lifetime. Document 28 requires "longer-lived"
// and rotation; thirty days matches a mobile app a driver uses daily without
// forcing a monthly re-login mid-shift.
const RefreshTokenTTL = 30 * 24 * time.Hour

// NewRefreshToken returns an opaque token and the hash to store.
//
// The token is random rather than signed: it is looked up in the database on
// every use anyway, so a signature would prove nothing extra, and randomness
// leaves nothing to forge.
func NewRefreshToken() (token string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("authn: generate refresh token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashRefreshToken(token), nil
}

// HashRefreshToken hashes a refresh token for storage and comparison.
//
// A plain SHA-256, not a password hash: the input is 256 bits of entropy this
// service generated, so there is no dictionary to attack and no reason to pay
// bcrypt's cost on every refresh.
func HashRefreshToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// --- OTP ---------------------------------------------------------------------

// OTPLength is six digits — long enough that guessing is impractical against
// the attempt limit, short enough to read off a screen and type.
const OTPLength = 6

// NewOTP returns a random numeric code.
func NewOTP() (string, error) {
	digits := make([]byte, OTPLength)
	for i := range digits {
		// crypto/rand, not math/rand: an OTP a caller can predict is not a
		// second factor.
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("authn: generate otp: %w", err)
		}
		digits[i] = byte('0' + n.Int64())
	}
	return string(digits), nil
}

// HashOTP keys the hash with the process secret and binds it to the phone it
// was issued for.
//
// Keying means a leaked database of hashes cannot be brute-forced offline — a
// six-digit code has only a million possibilities, so an unkeyed hash of one is
// equivalent to storing it in plaintext, which documents 28 and 123 forbid.
// Binding to the phone means a code issued for one number cannot be replayed
// against another.
func HashOTP(secret, phone, code string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(phone))
	mac.Write([]byte{0})
	mac.Write([]byte(code))
	return mac.Sum(nil)
}

// EqualOTP compares in constant time.
func EqualOTP(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
