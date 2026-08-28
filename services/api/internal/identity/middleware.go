package identity

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/sarmadkung/rideme/services/api/pkg/authn"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Document 28 requires two kinds of authorization, and they answer different
// questions:
//
//	role authorization      may this kind of actor do this at all?
//	resource authorization  may this actor do it to this object?
//
// A driver holds DRIVER, which is why they may edit a driver profile. It does
// not say which one. Both checks are needed, and only the first can live in
// middleware — the second needs the object, so it lives beside the handler
// that loaded it.

type contextKey int

const (
	principalKey contextKey = iota
)

// Principal is the authenticated caller.
type Principal struct {
	UserID    string
	SessionID string
	Roles     []Role
}

func (p Principal) HasRole(role Role) bool {
	for _, held := range p.Roles {
		if held == role {
			return true
		}
	}
	return false
}

func (p Principal) HasAnyRole(roles ...Role) bool {
	for _, role := range roles {
		if p.HasRole(role) {
			return true
		}
	}
	return false
}

// ContextWithPrincipal attaches a caller to a context.
//
// Authenticate is the only thing that should derive a principal from a
// request. This exists for the callers that already hold one and need to pass
// it down — internal work started on behalf of a user, and tests that exercise
// a handler without exercising the token pipeline as well.
func ContextWithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFrom returns the authenticated caller, if there is one.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// MustPrincipal returns the caller in a handler that sits behind Authenticate.
func MustPrincipal(ctx context.Context) Principal {
	p, _ := PrincipalFrom(ctx)
	return p
}

// Authenticate verifies the bearer token and attaches the principal.
//
// It verifies the token's signature only — no database round trip. That is
// what makes the access token short-lived: revocation takes effect at the next
// refresh, and the token's five-minute life is the bound on how stale an
// authorization can be.
func Authenticate(issuer *authn.Issuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
				return
			}
			claims, err := issuer.Verify(token)
			if err != nil {
				switch {
				case errors.Is(err, authn.ErrExpired):
					// Distinguished deliberately: a client that knows its
					// token expired refreshes silently instead of asking the
					// user to log in again.
					httpx.WriteError(w, r, httpx.Unauthorized("the access token has expired"))
				default:
					httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
				}
				return
			}

			roles := make([]Role, 0, len(claims.Roles))
			for _, role := range claims.Roles {
				roles = append(roles, Role(role))
			}
			ctx := context.WithValue(r.Context(), principalKey, Principal{
				UserID: claims.Subject, SessionID: claims.SessionID, Roles: roles,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole rejects a caller who holds none of the listed roles.
//
// It answers only "may this kind of actor do this". Ownership is the handler's
// job — see RequireSelfOrRole.
func RequireRole(roles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFrom(r.Context())
			if !ok {
				httpx.WriteError(w, r, httpx.Unauthorized("authentication required"))
				return
			}
			if !principal.HasAnyRole(roles...) {
				// Deliberately not "you need role X": that tells a caller how
				// the system is structured and what to go after.
				httpx.WriteError(w, r, httpx.Forbidden("not permitted"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireSelfOrRole is the resource check in its most common form: a caller may
// act on their own record, and a listed role may act on anyone's.
//
// This is the check document 28 illustrates with "driver can modify only their
// own driver profile".
func RequireSelfOrRole(principal Principal, ownerID string, roles ...Role) error {
	if principal.UserID != "" && principal.UserID == ownerID {
		return nil
	}
	if principal.HasAnyRole(roles...) {
		return nil
	}
	return httpx.Forbidden("not permitted")
}

// bearerToken extracts a token from the Authorization header (document 14:
// "Use Bearer authentication").
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "bearer") || token == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

// RequestContextFrom reads the device and network signals document 116 lists.
func RequestContextFrom(r *http.Request) RequestContext {
	return RequestContext{
		IP:         clientIP(r),
		DeviceID:   r.Header.Get("X-Device-Id"),
		Platform:   r.Header.Get("X-Platform"),
		OS:         r.Header.Get("X-OS"),
		AppVersion: r.Header.Get("X-App-Version"),
	}
}

// clientIP returns the peer address.
//
// X-Forwarded-For is deliberately ignored: it is client-supplied, so trusting
// it would let anyone forge the key that IP rate limiting counts against. When
// the platform runs behind a load balancer (Phase 14), the trusted-proxy
// configuration belongs there and this function changes with it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
