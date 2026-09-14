package config

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func env(overrides map[string]string) func(string) (string, bool) {
	base := map[string]string{
		"DATABASE_URL": "postgres://localhost:5432/logistics_dev",
		"REDIS_URL":    "redis://localhost:6379/0",
		"NATS_URL":     "nats://localhost:4222",
		"JWT_SECRET":   "test-secret",
	}
	for k, v := range overrides {
		base[k] = v
	}
	return func(key string) (string, bool) {
		v, ok := base[key]
		return v, ok
	}
}

func TestLoadAppliesDocumentedDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("expected valid configuration, got %v", err)
	}
	if cfg.Env != EnvDevelopment {
		t.Errorf("Env = %q, want development", cfg.Env)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080 (document 24)", cfg.Port)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	lookup := func(string) (string, bool) { return "", false }

	_, err := Load(lookup)
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
	}

	for _, want := range []string{"DATABASE_URL", "REDIS_URL", "NATS_URL", "JWT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("problem list is missing %s:\n%s", want, err.Error())
		}
	}
	if len(invalid.Problems) < 4 {
		t.Errorf("got %d problems, want all four missing variables reported together", len(invalid.Problems))
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"unknown environment": {"APP_ENV": "prod"},
		"non-numeric port":    {"API_PORT": "eighty-eighty"},
		"out-of-range port":   {"API_PORT": "70000"},
		"unknown log level":   {"LOG_LEVEL": "chatty"},
	}
	for name, overrides := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(env(overrides)); err == nil {
				t.Fatalf("expected validation failure for %v", overrides)
			}
		})
	}
}

func TestProductionRejectsThePlaceholderSecret(t *testing.T) {
	_, err := Load(env(map[string]string{
		"APP_ENV":    "production",
		"JWT_SECRET": "dev-only-change-me-in-every-other-environment",
	}))
	if err == nil {
		t.Fatal("production must not start with the development placeholder secret")
	}
}

func TestCORSAllowedOriginsDefaultsAndParses(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("expected valid configuration, got %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:5173" {
		t.Errorf("CORSAllowedOrigins = %v, want the admin dashboard's default port", cfg.CORSAllowedOrigins)
	}

	cfg, err = Load(env(map[string]string{
		"CORS_ALLOWED_ORIGINS": "https://admin.example.com, https://ops.example.com",
	}))
	if err != nil {
		t.Fatalf("expected valid configuration, got %v", err)
	}
	want := []string{"https://admin.example.com", "https://ops.example.com"}
	if len(cfg.CORSAllowedOrigins) != len(want) {
		t.Fatalf("CORSAllowedOrigins = %v, want %v", cfg.CORSAllowedOrigins, want)
	}
	for i, origin := range want {
		if cfg.CORSAllowedOrigins[i] != origin {
			t.Errorf("CORSAllowedOrigins[%d] = %q, want %q", i, cfg.CORSAllowedOrigins[i], origin)
		}
	}
}

func TestOTPBypassOnlyStartsInDevelopment(t *testing.T) {
	cfg, err := Load(env(map[string]string{"AUTH_OTP_BYPASS": "true"}))
	if err != nil {
		t.Fatalf("expected AUTH_OTP_BYPASS to be accepted in development, got %v", err)
	}
	if !cfg.OTPBypass {
		t.Error("OTPBypass = false, want true")
	}

	for _, otherEnv := range []string{"test", "staging", "production"} {
		t.Run(otherEnv, func(t *testing.T) {
			overrides := map[string]string{"APP_ENV": otherEnv, "AUTH_OTP_BYPASS": "true"}
			if otherEnv == "production" {
				overrides["JWT_SECRET"] = "a-real-production-secret"
			}
			if _, err := Load(env(overrides)); err == nil {
				t.Fatalf("AUTH_OTP_BYPASS must not start with APP_ENV=%s", otherEnv)
			}
		})
	}
}

func TestSelectingAMapProviderRequiresItsCredential(t *testing.T) {
	// Starting with MAP_PROVIDER=google and no key would send every route to
	// the straight-line fallback — the precise outcome setting the variable
	// was meant to end, and silently.
	_, err := Load(env(map[string]string{"MAP_PROVIDER": "google"}))
	if err == nil {
		t.Fatal("google was accepted with no MAPS_API_KEY")
	}
	if !strings.Contains(err.Error(), "MAPS_API_KEY") {
		t.Errorf("err = %v, want the missing variable named", err)
	}

	cfg, err := Load(env(map[string]string{
		"MAP_PROVIDER": "google",
		"MAPS_API_KEY": "a-key",
	}))
	if err != nil {
		t.Fatalf("google with a key was rejected: %v", err)
	}
	if cfg.MapsAPIKey != "a-key" {
		t.Errorf("MapsAPIKey = %q, want a-key", cfg.MapsAPIKey)
	}
}

func TestAnUnknownMapProviderIsRejected(t *testing.T) {
	// A typo must not fall through to the estimator as though nothing was
	// asked for.
	_, err := Load(env(map[string]string{"MAP_PROVIDER": "gooogle"}))
	if err == nil {
		t.Fatal("an unknown MAP_PROVIDER was accepted")
	}
	if !strings.Contains(err.Error(), "gooogle") {
		t.Errorf("err = %v, want the offending value quoted", err)
	}
}
