// Package config loads and validates process configuration exactly once, at
// startup (document 25). Business code receives a *Config; it never reads the
// environment directly.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env is the deployment environment. Document 23 fixes the four names.
type Env string

const (
	EnvDevelopment Env = "development"
	EnvTest        Env = "test"
	EnvStaging     Env = "staging"
	EnvProduction  Env = "production"
)

func (e Env) Valid() bool {
	switch e {
	case EnvDevelopment, EnvTest, EnvStaging, EnvProduction:
		return true
	}
	return false
}

func (e Env) IsProduction() bool { return e == EnvProduction }

type Config struct {
	Env      Env
	Port     int
	LogLevel slog.Level

	DatabaseURL string
	RedisURL    string
	NATSURL     string

	S3Endpoint  string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3Region    string

	// Phase 4. Validated now so a missing secret cannot reach production later.
	JWTSecret string

	OTPProvider string
	MapProvider string

	ShutdownTimeout time.Duration
	StartupTimeout  time.Duration
}

// ValidationError collects every problem rather than failing on the first, so a
// developer fixes their environment in one pass.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid configuration:\n  - %s", strings.Join(e.Problems, "\n  - "))
}

// Load reads from the given lookup function. os.LookupEnv in production; a map
// in tests.
func Load(lookup func(string) (string, bool)) (*Config, error) {
	l := loader{lookup: lookup}

	cfg := &Config{
		Env:             Env(l.optional("APP_ENV", string(EnvDevelopment))),
		Port:            l.optionalInt("API_PORT", 8080),
		DatabaseURL:     l.required("DATABASE_URL"),
		RedisURL:        l.required("REDIS_URL"),
		NATSURL:         l.required("NATS_URL"),
		S3Endpoint:      l.optional("S3_ENDPOINT", ""),
		S3Bucket:        l.optional("S3_BUCKET", ""),
		S3AccessKey:     l.optional("S3_ACCESS_KEY", ""),
		S3SecretKey:     l.optional("S3_SECRET_KEY", ""),
		S3Region:        l.optional("S3_REGION", "us-east-1"),
		JWTSecret:       l.required("JWT_SECRET"),
		OTPProvider:     l.optional("OTP_PROVIDER", "noop"),
		MapProvider:     l.optional("MAP_PROVIDER", "noop"),
		ShutdownTimeout: 15 * time.Second,
		StartupTimeout:  10 * time.Second,
	}
	cfg.LogLevel = parseLevel(l.optional("LOG_LEVEL", "info"), &l)

	if !cfg.Env.Valid() {
		l.problem("APP_ENV must be development, test, staging or production (got %q)", cfg.Env)
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		l.problem("API_PORT must be between 1 and 65535 (got %d)", cfg.Port)
	}
	if cfg.Env.IsProduction() && strings.HasPrefix(cfg.JWTSecret, "dev-only") {
		l.problem("JWT_SECRET still holds the development placeholder")
	}

	if len(l.problems) > 0 {
		return nil, &ValidationError{Problems: l.problems}
	}
	return cfg, nil
}

// LoadFromEnv is the production entry point.
func LoadFromEnv() (*Config, error) { return Load(os.LookupEnv) }

func parseLevel(raw string, l *loader) slog.Level {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		l.problem("LOG_LEVEL must be debug, info, warn or error (got %q)", raw)
		return slog.LevelInfo
	}
}

type loader struct {
	lookup   func(string) (string, bool)
	problems []string
}

func (l *loader) problem(format string, args ...any) {
	l.problems = append(l.problems, fmt.Sprintf(format, args...))
}

func (l *loader) required(key string) string {
	value, ok := l.lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		l.problem("%s is required", key)
		return ""
	}
	return value
}

func (l *loader) optional(key, fallback string) string {
	value, ok := l.lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (l *loader) optionalInt(key string, fallback int) int {
	raw, ok := l.lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		l.problem("%s must be an integer (got %q)", key, raw)
		return fallback
	}
	return value
}

// MigrationConfig is the configuration the migrate command needs.
//
// Deliberately narrow: a schema migration has no business requiring a JWT
// secret or an S3 bucket, and demanding them would make the tool unusable in a
// deploy job that legitimately has neither.
type MigrationConfig struct {
	DatabaseURL string
	SourcePath  string
}

func LoadMigrationConfig(lookup func(string) (string, bool)) (*MigrationConfig, error) {
	l := loader{lookup: lookup}
	cfg := &MigrationConfig{
		DatabaseURL: l.required("DATABASE_URL"),
		SourcePath:  l.optional("MIGRATIONS_PATH", "migrations"),
	}
	if len(l.problems) > 0 {
		return nil, &ValidationError{Problems: l.problems}
	}
	return cfg, nil
}

func LoadMigrationConfigFromEnv() (*MigrationConfig, error) {
	return LoadMigrationConfig(os.LookupEnv)
}
