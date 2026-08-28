//go:build integration

package tests

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/pkg/authn"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/notify"
	"github.com/sarmadkung/rideme/services/api/pkg/ratelimit"
)

// Authentication is a Level 5 area: verification-lite escalates it regardless
// of diff size. These tests run against the real database because the
// properties that matter — single-use codes, refresh rotation, reuse detection
// — are enforced by SQL, and a mocked store would assert nothing about them.

const testSecret = "integration-secret-at-least-32-bytes-long"

type harness struct {
	service *identity.Service
	store   *identity.Store
	sender  *notify.MemorySender
	pool    *pgxpool.Pool
	clock   time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)

	h := &harness{
		store:  identity.NewStore(pool),
		sender: notify.NewMemorySender(),
		pool:   pool,
		clock:  time.Now().UTC(),
	}
	issuer, err := authn.NewIssuer(testSecret, authn.WithClock(func() time.Time { return h.clock }))
	if err != nil {
		t.Fatal(err)
	}
	h.service = identity.NewService(
		h.store, issuer, h.sender, ratelimit.NewMemoryLimiter(),
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		testSecret,
		identity.Options{Now: func() time.Time { return h.clock }},
	)
	return h
}

// uniquePhone keeps runs independent without truncating shared tables.
func uniquePhone(t *testing.T) string {
	t.Helper()
	// A valid Pakistani mobile number derived from the clock, so repeated runs
	// do not collide on the users.phone unique constraint.
	n := time.Now().UnixNano() % 1_000_000_0
	phone := "03" + strings.Repeat("0", 9) // 11 digits, national form
	suffix := []byte(phone)
	digits := []byte(strings.Repeat("0", 8))
	for i := len(digits) - 1; i >= 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	copy(suffix[3:], digits)
	return string(suffix)
}

// lastCode reads the OTP out of the message the sender recorded. Nothing else
// in the system can read it back — the stored form is a keyed hash.
func (h *harness) lastCode(t *testing.T) string {
	t.Helper()
	msg, ok := h.sender.Last()
	if !ok {
		t.Fatal("no message was sent")
	}
	fields := strings.Fields(msg.Body)
	if len(fields) == 0 {
		t.Fatalf("unexpected message body: %q", msg.Body)
	}
	return fields[0]
}

func (h *harness) login(t *testing.T, phone string) identity.Tokens {
	t.Helper()
	ctx := context.Background()
	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{IP: "127.0.0.1"}); err != nil {
		t.Fatalf("request otp: %v", err)
	}
	tokens, err := h.service.VerifyOTP(ctx, phone, h.lastCode(t), identity.PurposeLogin,
		identity.RequestContext{IP: "127.0.0.1", DeviceID: "device-1", Platform: "ios"})
	if err != nil {
		t.Fatalf("verify otp: %v", err)
	}
	return tokens
}

func codeOf(t *testing.T, err error) httpx.Code {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	return httpx.AsError(err).Code
}

// --- the documented flow -----------------------------------------------------

func TestOTPLoginCreatesAnAccountAndIssuesTokens(t *testing.T) {
	h := newHarness(t)
	phone := uniquePhone(t)

	tokens := h.login(t, phone)

	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("login did not issue both tokens")
	}
	// Document 28: a new caller becomes a CUSTOMER. Nothing self-assigns more.
	if len(tokens.User.Roles) != 1 || tokens.User.Roles[0] != identity.RoleCustomer {
		t.Fatalf("new account roles = %v, want [CUSTOMER]", tokens.User.Roles)
	}
	// The phone is stored normalised, not as typed.
	if !strings.HasPrefix(tokens.User.Phone, "+92") {
		t.Fatalf("phone %q was not normalised to E.164", tokens.User.Phone)
	}

	// Logging in again resolves the same account rather than creating another.
	second := h.login(t, phone)
	if second.User.ID != tokens.User.ID {
		t.Fatalf("second login created a new account: %s then %s", tokens.User.ID, second.User.ID)
	}
}

func TestTheSameNumberInAnyFormatReachesOneAccount(t *testing.T) {
	h := newHarness(t)
	national := uniquePhone(t)
	international := "+92" + national[1:]

	first := h.login(t, national)
	second := h.login(t, international)

	if first.User.ID != second.User.ID {
		t.Fatalf("0-prefixed and +92 forms created two accounts: %s and %s", first.User.ID, second.User.ID)
	}
}

// --- OTP properties ----------------------------------------------------------

func TestTheCodeIsNeverStoredInPlaintext(t *testing.T) {
	// Documents 28 and 123 both require this. A leaked table must not be a
	// list of usable codes.
	h := newHarness(t)
	phone := uniquePhone(t)
	ctx := context.Background()

	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	code := h.lastCode(t)

	var stored []byte
	err := h.pool.QueryRow(ctx,
		`SELECT code_hash FROM otp_challenges WHERE phone = $1 AND consumed_at IS NULL`,
		"+92"+phone[1:]).Scan(&stored)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), code) {
		t.Fatal("the stored hash contains the code")
	}
	if len(stored) != 32 {
		t.Fatalf("stored hash is %d bytes, want a 32-byte HMAC", len(stored))
	}
}

func TestACodeWorksExactlyOnce(t *testing.T) {
	h := newHarness(t)
	phone := uniquePhone(t)
	ctx := context.Background()

	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	code := h.lastCode(t)

	if _, err := h.service.VerifyOTP(ctx, phone, code, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatalf("first use failed: %v", err)
	}
	// Replaying a code that already logged someone in must not log them in
	// again — single-use is a property of the OTP (document 123).
	_, err := h.service.VerifyOTP(ctx, phone, code, identity.PurposeLogin, identity.RequestContext{})
	if got := codeOf(t, err); got != httpx.CodeUnauthorized {
		t.Fatalf("replay returned %q, want unauthorized", got)
	}
}

func TestConcurrentVerificationsOfOneCodeProduceOneLogin(t *testing.T) {
	// Single-use has to hold under a race, not just in sequence. Two clients
	// submitting the same correct code simultaneously must not both succeed.
	h := newHarness(t)
	phone := uniquePhone(t)
	ctx := context.Background()

	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	code := h.lastCode(t)

	const racers = 8
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start
			_, err := h.service.VerifyOTP(ctx, phone, code, identity.PurposeLogin, identity.RequestContext{})
			results <- err
		}()
	}
	close(start)

	var succeeded int
	for i := 0; i < racers; i++ {
		if err := <-results; err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of %d concurrent verifications succeeded, want exactly 1", succeeded, racers)
	}
}

func TestAWrongCodeIsRejectedAndAttemptsAreBounded(t *testing.T) {
	h := newHarness(t)
	phone := uniquePhone(t)
	ctx := context.Background()

	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	realCode := h.lastCode(t)
	wrong := "000000"
	if realCode == wrong {
		wrong = "111111"
	}

	for i := 0; i < identity.DefaultMaxAttempts; i++ {
		_, err := h.service.VerifyOTP(ctx, phone, wrong, identity.PurposeLogin, identity.RequestContext{})
		if got := codeOf(t, err); got != httpx.CodeUnauthorized {
			t.Fatalf("attempt %d returned %q", i+1, got)
		}
	}
	// Past the attempt limit even the correct code must fail: otherwise the
	// limit only slows an attacker down rather than stopping them.
	_, err := h.service.VerifyOTP(ctx, phone, realCode, identity.PurposeLogin, identity.RequestContext{})
	if got := codeOf(t, err); got != httpx.CodeUnauthorized {
		t.Fatalf("the correct code succeeded after the attempt limit (%q)", got)
	}
}

func TestAnExpiredCodeIsRejected(t *testing.T) {
	h := newHarness(t)
	phone := uniquePhone(t)
	ctx := context.Background()

	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	code := h.lastCode(t)

	h.clock = h.clock.Add(identity.DefaultOTPTTL + time.Second)

	_, err := h.service.VerifyOTP(ctx, phone, code, identity.PurposeLogin, identity.RequestContext{})
	if got := codeOf(t, err); got != httpx.CodeUnauthorized {
		t.Fatalf("an expired code returned %q, want unauthorized", got)
	}
}

func TestACodeIsBoundToItsPurpose(t *testing.T) {
	// Document 123: OTPs are purpose-bound. A login code must not authorise a
	// phone change.
	h := newHarness(t)
	phone := uniquePhone(t)
	ctx := context.Background()

	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	code := h.lastCode(t)

	_, err := h.service.VerifyOTP(ctx, phone, code, identity.PurposePhoneChange, identity.RequestContext{})
	if got := codeOf(t, err); got != httpx.CodeUnauthorized {
		t.Fatalf("a login code verified against PHONE_CHANGE (%q)", got)
	}
}

func TestRequestingASecondCodeInvalidatesTheFirst(t *testing.T) {
	h := newHarness(t)
	phone := uniquePhone(t)
	ctx := context.Background()

	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	first := h.lastCode(t)
	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	second := h.lastCode(t)

	if _, err := h.service.VerifyOTP(ctx, phone, first, identity.PurposeLogin, identity.RequestContext{}); err == nil {
		t.Fatal("the superseded code still worked")
	}
	if _, err := h.service.VerifyOTP(ctx, phone, second, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatalf("the current code failed: %v", err)
	}
}

func TestUnknownAndKnownNumbersAreIndistinguishable(t *testing.T) {
	// Document 28: prevent account enumeration. The response for a number with
	// an account must match one without.
	h := newHarness(t)
	ctx := context.Background()

	known := uniquePhone(t)
	h.login(t, known)
	unknown := uniquePhone(t)

	knownExpiry, err := h.service.RequestOTP(ctx, known, identity.PurposeLogin, identity.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	unknownExpiry, err := h.service.RequestOTP(ctx, unknown, identity.PurposeLogin, identity.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !knownExpiry.Equal(unknownExpiry) {
		t.Fatalf("expiry differs between a known and unknown number: %v vs %v", knownExpiry, unknownExpiry)
	}

	// And a wrong code fails identically for both.
	knownErr := codeOf(t, mustFail(t, h, known))
	unknownErr := codeOf(t, mustFail(t, h, unknown))
	if knownErr != unknownErr {
		t.Fatalf("error codes differ: %q vs %q", knownErr, unknownErr)
	}
}

func mustFail(t *testing.T, h *harness, phone string) error {
	t.Helper()
	_, err := h.service.VerifyOTP(context.Background(), phone, "999999", identity.PurposeLogin, identity.RequestContext{})
	if err == nil {
		t.Fatal("expected the wrong code to fail")
	}
	return err
}

// --- sessions and refresh ----------------------------------------------------

func TestRefreshRotatesTheTokenAndTheOldOneStopsWorking(t *testing.T) {
	h := newHarness(t)
	tokens := h.login(t, uniquePhone(t))
	ctx := context.Background()

	rotated, err := h.service.Refresh(ctx, tokens.RefreshToken, identity.RequestContext{})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if rotated.RefreshToken == tokens.RefreshToken {
		t.Fatal("the refresh token was not rotated")
	}
	if rotated.Session.ID != tokens.Session.ID {
		t.Fatal("rotation opened a new session instead of continuing the old one")
	}
	if rotated.AccessToken == "" {
		t.Fatal("refresh did not issue an access token")
	}
}

func TestReusingARotatedRefreshTokenRevokesEverySession(t *testing.T) {
	// This is the point of rotation. A stolen token works once; when the real
	// client next refreshes, the mismatch is the evidence of theft, and the
	// safe response is to end every session and force a fresh login.
	h := newHarness(t)
	tokens := h.login(t, uniquePhone(t))
	ctx := context.Background()

	rotated, err := h.service.Refresh(ctx, tokens.RefreshToken, identity.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}

	// The attacker replays the token they captured.
	if _, err := h.service.Refresh(ctx, tokens.RefreshToken, identity.RequestContext{}); err == nil {
		t.Fatal("a reused refresh token was accepted")
	}

	// And the legitimate client's newer token is now dead too.
	if _, err := h.service.Refresh(ctx, rotated.RefreshToken, identity.RequestContext{}); err == nil {
		t.Fatal("sessions were not revoked after reuse was detected")
	}

	sessions, err := h.service.Sessions(ctx, tokens.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("%d sessions survived reuse detection, want 0", len(sessions))
	}
}

func TestConcurrentRefreshesIssueOneLiveToken(t *testing.T) {
	h := newHarness(t)
	tokens := h.login(t, uniquePhone(t))
	ctx := context.Background()

	const racers = 6
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start
			_, err := h.service.Refresh(ctx, tokens.RefreshToken, identity.RequestContext{})
			results <- err
		}()
	}
	close(start)

	var succeeded int
	for i := 0; i < racers; i++ {
		if err := <-results; err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of %d concurrent refreshes succeeded, want exactly 1", succeeded, racers)
	}
}

func TestLogoutEndsTheSessionAndRefreshStops(t *testing.T) {
	h := newHarness(t)
	tokens := h.login(t, uniquePhone(t))
	ctx := context.Background()

	if err := h.service.Logout(ctx, tokens.Session.ID, tokens.User.ID, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Refresh(ctx, tokens.RefreshToken, identity.RequestContext{}); err == nil {
		t.Fatal("refresh worked after logout")
	}
}

func TestLogoutAllEndsEverySession(t *testing.T) {
	h := newHarness(t)
	phone := uniquePhone(t)
	first := h.login(t, phone)
	second := h.login(t, phone)
	ctx := context.Background()

	sessions, err := h.service.Sessions(ctx, first.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) < 2 {
		t.Fatalf("expected at least two live sessions, got %d", len(sessions))
	}

	if _, err := h.service.LogoutAll(ctx, first.User.ID, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	for name, token := range map[string]string{"first": first.RefreshToken, "second": second.RefreshToken} {
		if _, err := h.service.Refresh(ctx, token, identity.RequestContext{}); err == nil {
			t.Fatalf("the %s session survived logout-all", name)
		}
	}
}

func TestAnExpiredSessionCannotRefresh(t *testing.T) {
	h := newHarness(t)
	tokens := h.login(t, uniquePhone(t))
	ctx := context.Background()

	h.clock = h.clock.Add(authn.RefreshTokenTTL + time.Hour)

	if _, err := h.service.Refresh(ctx, tokens.RefreshToken, identity.RequestContext{}); err == nil {
		t.Fatal("an expired session refreshed")
	}
}

// --- authorization -----------------------------------------------------------

func TestASuspendedAccountCannotAuthenticateOrRefresh(t *testing.T) {
	h := newHarness(t)
	phone := uniquePhone(t)
	tokens := h.login(t, phone)
	ctx := context.Background()

	if _, err := h.pool.Exec(ctx, `UPDATE users SET status = 'SUSPENDED' WHERE id = $1`, tokens.User.ID); err != nil {
		t.Fatal(err)
	}

	// An existing session stops refreshing...
	err := codeOf(t, mustRefreshFail(t, h, tokens.RefreshToken))
	if err != httpx.CodeForbidden {
		t.Fatalf("refresh on a suspended account returned %q, want forbidden", err)
	}

	// ...and a fresh login is refused too.
	if _, rerr := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); rerr != nil {
		t.Fatal(rerr)
	}
	_, verr := h.service.VerifyOTP(ctx, phone, h.lastCode(t), identity.PurposeLogin, identity.RequestContext{})
	if got := codeOf(t, verr); got != httpx.CodeForbidden {
		t.Fatalf("login on a suspended account returned %q, want forbidden", got)
	}
}

func mustRefreshFail(t *testing.T, h *harness, token string) error {
	t.Helper()
	_, err := h.service.Refresh(context.Background(), token, identity.RequestContext{})
	if err == nil {
		t.Fatal("expected refresh to fail")
	}
	return err
}

func TestRolesAreReReadOnRefreshSoRevocationTakesEffect(t *testing.T) {
	// An access token carries roles, so a revoked role keeps working until the
	// token expires. It must not survive the refresh after that.
	h := newHarness(t)
	tokens := h.login(t, uniquePhone(t))
	ctx := context.Background()

	if err := h.store.GrantRole(ctx, tokens.User.ID, identity.RoleDriver); err != nil {
		t.Fatal(err)
	}
	granted, err := h.service.Refresh(ctx, tokens.RefreshToken, identity.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !granted.User.HasRole(identity.RoleDriver) {
		t.Fatal("a granted role did not appear after refresh")
	}

	if err := h.store.RevokeRole(ctx, tokens.User.ID, identity.RoleDriver); err != nil {
		t.Fatal(err)
	}
	revoked, err := h.service.Refresh(ctx, granted.RefreshToken, identity.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	if revoked.User.HasRole(identity.RoleDriver) {
		t.Fatal("a revoked role survived refresh")
	}
}

func TestResourceAuthorizationLetsOwnersThroughAndStopsStrangers(t *testing.T) {
	// Document 28's example: "driver can modify only their own driver profile".
	owner := identity.Principal{UserID: "user-1", Roles: []identity.Role{identity.RoleDriver}}
	stranger := identity.Principal{UserID: "user-2", Roles: []identity.Role{identity.RoleDriver}}
	support := identity.Principal{UserID: "user-3", Roles: []identity.Role{identity.RoleSupport}}

	if err := identity.RequireSelfOrRole(owner, "user-1", identity.RoleAdmin); err != nil {
		t.Fatalf("the owner was refused: %v", err)
	}
	// Holding DRIVER is not enough to touch another driver's record — which is
	// exactly what role authorization alone would have allowed.
	if err := identity.RequireSelfOrRole(stranger, "user-1", identity.RoleAdmin); err == nil {
		t.Fatal("another driver was allowed through")
	}
	if err := identity.RequireSelfOrRole(support, "user-1", identity.RoleSupport); err != nil {
		t.Fatalf("a permitted role was refused: %v", err)
	}
}

func TestRateLimitingStopsOTPFlooding(t *testing.T) {
	h := newHarness(t)
	phone := uniquePhone(t)
	ctx := context.Background()

	var limited error
	for i := 0; i < identity.RuleOTPPerPhone.Limit+1; i++ {
		_, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{IP: "10.0.0.1"})
		if err != nil {
			limited = err
			break
		}
	}
	if limited == nil {
		t.Fatal("OTP requests were never limited")
	}
	if got := codeOf(t, limited); got != httpx.CodeRateLimited {
		t.Fatalf("limit returned %q, want rate_limited", got)
	}
}

// --- audit -------------------------------------------------------------------

func TestSecurityEventsAreRecorded(t *testing.T) {
	// Document 28 lists what must be audited. An audit trail that is missing
	// the login it is meant to explain is worse than none.
	h := newHarness(t)
	tokens := h.login(t, uniquePhone(t))
	ctx := context.Background()

	if err := h.service.Logout(ctx, tokens.Session.ID, tokens.User.ID, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}

	rows, err := h.pool.Query(ctx,
		`SELECT event FROM security_events WHERE user_id = $1 ORDER BY created_at`, tokens.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var event string
		if err := rows.Scan(&event); err != nil {
			t.Fatal(err)
		}
		seen[event] = true
	}
	for _, want := range []identity.SecurityEvent{identity.EventLogin, identity.EventLogout} {
		if !seen[string(want)] {
			t.Errorf("%s was not audited", want)
		}
	}
}

func TestAuditNeverStoresAWholePhoneNumber(t *testing.T) {
	h := newHarness(t)
	phone := uniquePhone(t)
	ctx := context.Background()

	if _, err := h.service.RequestOTP(ctx, phone, identity.PurposeLogin, identity.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	normalised := "+92" + phone[1:]

	var count int
	err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM security_events WHERE metadata->>'phone' = $1`, normalised).Scan(&count)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("%d audit rows contain the full phone number; it should be masked", count)
	}
}
