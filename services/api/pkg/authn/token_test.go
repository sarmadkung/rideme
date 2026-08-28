package authn_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/authn"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestIssuerRejectsAWeakSecret(t *testing.T) {
	if _, err := authn.NewIssuer("short"); err == nil {
		t.Fatal("accepted a secret too short to sign with")
	}
}

func TestIssueAndVerifyRoundTrip(t *testing.T) {
	issuer, err := authn.NewIssuer(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	token, issued, err := issuer.Issue("user-1", "session-1", []string{"CUSTOMER", "DRIVER"})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := issuer.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" || claims.SessionID != "session-1" {
		t.Fatalf("claims did not survive the round trip: %+v", claims)
	}
	if len(claims.Roles) != 2 || claims.Roles[0] != "CUSTOMER" {
		t.Fatalf("roles did not survive: %+v", claims.Roles)
	}
	if claims.ExpiresAt != issued.ExpiresAt {
		t.Fatal("verified expiry differs from issued expiry")
	}
}

func TestVerifyRejectsATamperedPayload(t *testing.T) {
	// The whole point of signing: editing the claims must invalidate the
	// token. A forged `roles` claim would otherwise be privilege escalation.
	issuer, _ := authn.NewIssuer(testSecret)
	token, _, err := issuer.Issue("user-1", "session-1", []string{"CUSTOMER"})
	if err != nil {
		t.Fatal(err)
	}
	body, signature, _ := strings.Cut(token, ".")

	forged := body[:len(body)-1] + "A" + "." + signature
	if _, err := issuer.Verify(forged); !errors.Is(err, authn.ErrSignature) {
		t.Fatalf("want ErrSignature, got %v", err)
	}
}

func TestVerifyRejectsATokenFromAnotherSecret(t *testing.T) {
	mint, _ := authn.NewIssuer(testSecret)
	other, _ := authn.NewIssuer("fedcba9876543210fedcba9876543210")

	token, _, err := mint.Issue("user-1", "session-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Verify(token); !errors.Is(err, authn.ErrSignature) {
		t.Fatalf("want ErrSignature, got %v", err)
	}
}

func TestVerifyRejectsMalformedTokens(t *testing.T) {
	issuer, _ := authn.NewIssuer(testSecret)
	for _, token := range []string{"", ".", "nodot", "body.", ".signature", "!!!.???"} {
		if _, err := issuer.Verify(token); err == nil {
			t.Errorf("%q was accepted", token)
		}
	}
}

func TestVerifyRejectsAnExpiredToken(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	clock := now
	issuer, _ := authn.NewIssuer(testSecret, authn.WithClock(func() time.Time { return clock }))

	token, _, err := issuer.Issue("user-1", "session-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Verify(token); err != nil {
		t.Fatalf("valid at issue time: %v", err)
	}

	clock = now.Add(authn.AccessTokenTTL - time.Second)
	if _, err := issuer.Verify(token); err != nil {
		t.Fatalf("should still be valid one second before expiry: %v", err)
	}

	clock = now.Add(authn.AccessTokenTTL)
	if _, err := issuer.Verify(token); !errors.Is(err, authn.ErrExpired) {
		t.Fatalf("want ErrExpired at the boundary, got %v", err)
	}
}

func TestRefreshTokensAreUniqueAndHashConsistently(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		token, hash, err := authn.NewRefreshToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[token] {
			t.Fatal("generated a duplicate refresh token")
		}
		seen[token] = true

		if string(authn.HashRefreshToken(token)) != string(hash) {
			t.Fatal("hashing the token did not reproduce the stored hash")
		}
		if strings.Contains(string(hash), token) {
			t.Fatal("the hash contains the token")
		}
	}
}

func TestOTPIsSixRandomDigits(t *testing.T) {
	seen := map[string]int{}
	for i := 0; i < 200; i++ {
		code, err := authn.NewOTP()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != authn.OTPLength {
			t.Fatalf("want %d digits, got %q", authn.OTPLength, code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("non-digit in %q", code)
			}
		}
		seen[code]++
	}
	// 200 draws from a million values should essentially never repeat; a
	// generator stuck on one value is the failure this catches.
	if len(seen) < 190 {
		t.Fatalf("only %d distinct codes in 200 draws — generator looks weak", len(seen))
	}
}

func TestOTPHashIsBoundToThePhoneAndTheSecret(t *testing.T) {
	const (
		phoneA = "+923001234567"
		phoneB = "+923009999999"
		code   = "123456"
	)
	base := authn.HashOTP(testSecret, phoneA, code)

	if !authn.EqualOTP(base, authn.HashOTP(testSecret, phoneA, code)) {
		t.Fatal("hashing is not deterministic")
	}
	// A code issued for one number must not verify against another.
	if authn.EqualOTP(base, authn.HashOTP(testSecret, phoneB, code)) {
		t.Fatal("hash is not bound to the phone number")
	}
	if authn.EqualOTP(base, authn.HashOTP("another-secret-of-sufficient-len", phoneA, code)) {
		t.Fatal("hash is not keyed by the secret")
	}
	if authn.EqualOTP(base, authn.HashOTP(testSecret, phoneA, "654321")) {
		t.Fatal("different codes produced the same hash")
	}
}
