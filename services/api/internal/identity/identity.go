// Package identity is the platform's shared account system (documents 20, 28):
// one user record behind every customer, driver, merchant, support agent and
// administrator.
//
// One table, not five. Document 28 states a user may hold more than one role,
// and in this market that is the common case rather than the exception — a
// driver orders groceries, a merchant books a parcel. Separate account systems
// per actor would make that person several people, with several phone numbers
// of record and no way to reconcile them.
package identity

import (
	"errors"
	"time"
)

// Role is one of the six document 20 fixes.
type Role string

const (
	RoleCustomer   Role = "CUSTOMER"
	RoleDriver     Role = "DRIVER"
	RoleMerchant   Role = "MERCHANT"
	RoleSupport    Role = "SUPPORT"
	RoleAdmin      Role = "ADMIN"
	RoleSuperAdmin Role = "SUPER_ADMIN"
)

// AllRoles is the closed set, in the order document 20 lists them.
var AllRoles = []Role{RoleCustomer, RoleDriver, RoleMerchant, RoleSupport, RoleAdmin, RoleSuperAdmin}

func (r Role) Valid() bool {
	for _, known := range AllRoles {
		if r == known {
			return true
		}
	}
	return false
}

// Status is the account lifecycle.
type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusSuspended Status = "SUSPENDED"
	StatusDeleted   Status = "DELETED"
)

// User is a person, whatever they do on the platform.
type User struct {
	ID        string
	Phone     string
	Email     string
	Name      string
	AvatarURL string
	Status    Status
	Roles     []Role
	CreatedAt time.Time
	UpdatedAt time.Time
}

// HasRole reports whether the user holds a role.
func (u User) HasRole(role Role) bool {
	for _, held := range u.Roles {
		if held == role {
			return true
		}
	}
	return false
}

// HasAnyRole reports whether the user holds at least one of the roles.
func (u User) HasAnyRole(roles ...Role) bool {
	for _, role := range roles {
		if u.HasRole(role) {
			return true
		}
	}
	return false
}

// RoleStrings renders roles for a token claim.
func (u User) RoleStrings() []string {
	out := make([]string, len(u.Roles))
	for i, role := range u.Roles {
		out[i] = string(role)
	}
	return out
}

// CanAuthenticate reports whether this account may be issued a session. A
// suspended account keeps its data and loses its access.
func (u User) CanAuthenticate() bool { return u.Status == StatusActive }

// Device is a client installation (document 116). The signals are deliberately
// few — that document warns against collecting unnecessary fingerprinting data.
type Device struct {
	ID         string
	UserID     string
	Identifier string
	Platform   string
	OS         string
	AppVersion string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// Session is an authenticated login, carrying exactly the fields document 28
// lists. The refresh token itself is never held here — only its hash, in the
// database.
type Session struct {
	ID            string
	UserID        string
	DeviceID      string
	CreatedAt     time.Time
	LastUsedAt    time.Time
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	RevokedReason string
}

// Active reports whether the session may still be refreshed.
func (s Session) Active(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

// OTPPurpose binds a code to what it was issued for, so a login code cannot be
// replayed to authorise a phone change (documents 28, 123: OTPs are
// "purpose-bound").
type OTPPurpose string

const (
	PurposeLogin       OTPPurpose = "LOGIN"
	PurposePhoneChange OTPPurpose = "PHONE_CHANGE"
	PurposeStepUp      OTPPurpose = "STEP_UP"
)

// Challenge is an outstanding OTP.
type Challenge struct {
	ID          string
	Phone       string
	Purpose     OTPPurpose
	CodeHash    []byte
	Attempts    int
	MaxAttempts int
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
	CreatedAt   time.Time
}

// Usable reports whether the challenge can still be verified against.
func (c Challenge) Usable(now time.Time) bool {
	return c.ConsumedAt == nil && c.Attempts < c.MaxAttempts && now.Before(c.ExpiresAt)
}

// SecurityEvent is an audit record. Document 28 lists what must be audited;
// these constants are that list.
type SecurityEvent string

const (
	EventLogin            SecurityEvent = "auth.login"
	EventLogout           SecurityEvent = "auth.logout"
	EventLogoutAll        SecurityEvent = "auth.logout_all"
	EventOTPRequested     SecurityEvent = "auth.otp_requested"
	EventOTPFailed        SecurityEvent = "auth.otp_failed"
	EventRefreshed        SecurityEvent = "auth.refreshed"
	EventRefreshReuse     SecurityEvent = "auth.refresh_reuse"
	EventRoleChanged      SecurityEvent = "auth.role_changed"
	EventPhoneChanged     SecurityEvent = "auth.phone_changed"
	EventRateLimited      SecurityEvent = "auth.rate_limited"
	EventSuspiciousDevice SecurityEvent = "auth.suspicious_device"
	EventAccountSuspended SecurityEvent = "auth.account_suspended"
)

// Audit is one record to append.
type Audit struct {
	UserID    string
	SessionID string
	DeviceID  string
	Event     SecurityEvent
	IP        string
	Metadata  map[string]any
}

var (
	ErrUserNotFound      = errors.New("identity: user not found")
	ErrSessionNotFound   = errors.New("identity: session not found")
	ErrChallengeNotFound = errors.New("identity: no outstanding challenge")
	ErrInvalidRole       = errors.New("identity: unknown role")
)
