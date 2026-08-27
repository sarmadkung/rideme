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
