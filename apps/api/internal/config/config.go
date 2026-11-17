// Package config loads and validates the runtime configuration from the
// environment. Validation happens at startup so that misconfiguration fails
// fast instead of surfacing as a confusing runtime error.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Environment identifies the deployment context the process runs in.
type Environment string

const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentTest        Environment = "test"
	EnvironmentProduction  Environment = "production"
)

// Config is the fully validated runtime configuration.
type Config struct {
	AttachmentDir     string
	Environment       Environment
	HTTPAddr          string
	DatabaseURL       string
	DBMaxConns        int32
	DBMinConns        int32
	DBConnectTimeout  time.Duration
	DBMaxConnLifetime time.Duration
	DBMaxConnIdleTime time.Duration
	ShutdownTimeout   time.Duration
	MigrateOnStart    bool
	LogLevel          slog.Level
	SessionTTL        time.Duration
	LoginMaxFailures  int
	LoginWindow       time.Duration
}

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(lookup func(string) string) (Config, error) {
	cfg := Config{
		Environment:       EnvironmentDevelopment,
		AttachmentDir:     ".data/attachments",
		HTTPAddr:          "127.0.0.1:8080",
		DBMaxConns:        10,
		DBMinConns:        0,
		DBConnectTimeout:  10 * time.Second,
		DBMaxConnLifetime: time.Hour,
		DBMaxConnIdleTime: 30 * time.Minute,
		ShutdownTimeout:   15 * time.Second,
		MigrateOnStart:    false,
		LogLevel:          slog.LevelInfo,
		SessionTTL:        12 * time.Hour,
		LoginMaxFailures:  10,
		LoginWindow:       15 * time.Minute,
	}

	if raw := strings.TrimSpace(lookup("NEXORA_ENV")); raw != "" {
		env := Environment(strings.ToLower(raw))
		switch env {
		case EnvironmentDevelopment, EnvironmentTest, EnvironmentProduction:
			cfg.Environment = env
		default:
			return Config{}, fmt.Errorf("NEXORA_ENV must be one of development, test, production (got %q)", raw)
		}
	}

	if raw := strings.TrimSpace(lookup("NEXORA_ATTACHMENT_DIR")); raw != "" {
		cfg.AttachmentDir = raw
	}
	if raw := strings.TrimSpace(lookup("NEXORA_HTTP_ADDR")); raw != "" {
		cfg.HTTPAddr = raw
	}
	cfg.DatabaseURL = strings.TrimSpace(lookup("NEXORA_DATABASE_URL"))
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("NEXORA_DATABASE_URL is required")
	}
	if err := validateDatabaseURL(cfg.DatabaseURL); err != nil {
		return Config{}, err
	}

	var err error
	if cfg.DBMaxConns, err = int32FromEnv(lookup, "NEXORA_DB_MAX_CONNS", cfg.DBMaxConns); err != nil {
		return Config{}, err
	}
	if cfg.DBMinConns, err = int32FromEnv(lookup, "NEXORA_DB_MIN_CONNS", cfg.DBMinConns); err != nil {
		return Config{}, err
	}
	if cfg.DBConnectTimeout, err = durationFromEnv(lookup, "NEXORA_DB_CONNECT_TIMEOUT", cfg.DBConnectTimeout); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxConnLifetime, err = durationFromEnv(lookup, "NEXORA_DB_MAX_CONN_LIFETIME", cfg.DBMaxConnLifetime); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxConnIdleTime, err = durationFromEnv(lookup, "NEXORA_DB_MAX_CONN_IDLE_TIME", cfg.DBMaxConnIdleTime); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = durationFromEnv(lookup, "NEXORA_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.MigrateOnStart, err = boolFromEnv(lookup, "NEXORA_MIGRATE_ON_START", cfg.MigrateOnStart); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = logLevelFromEnv(lookup, cfg.LogLevel); err != nil {
		return Config{}, err
	}
	if cfg.SessionTTL, err = durationFromEnv(lookup, "NEXORA_SESSION_TTL", cfg.SessionTTL); err != nil {
		return Config{}, err
	}
	if cfg.LoginWindow, err = durationFromEnv(lookup, "NEXORA_LOGIN_WINDOW", cfg.LoginWindow); err != nil {
		return Config{}, err
	}
	if raw := strings.TrimSpace(lookup("NEXORA_LOGIN_MAX_FAILURES")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return Config{}, fmt.Errorf("NEXORA_LOGIN_MAX_FAILURES must be an integer: %w", parseErr)
		}
		cfg.LoginMaxFailures = value
	}

	if cfg.DBMaxConns < 1 {
		return Config{}, fmt.Errorf("NEXORA_DB_MAX_CONNS must be at least 1 (got %d)", cfg.DBMaxConns)
	}
	if cfg.DBMinConns < 0 || cfg.DBMinConns > cfg.DBMaxConns {
		return Config{}, fmt.Errorf("NEXORA_DB_MIN_CONNS must be between 0 and NEXORA_DB_MAX_CONNS (got %d)", cfg.DBMinConns)
	}
	if cfg.DBConnectTimeout <= 0 {
		return Config{}, fmt.Errorf("NEXORA_DB_CONNECT_TIMEOUT must be positive")
	}
	if cfg.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("NEXORA_SHUTDOWN_TIMEOUT must be positive")
	}
	if cfg.Environment == EnvironmentProduction && cfg.MigrateOnStart {
		return Config{}, fmt.Errorf("NEXORA_MIGRATE_ON_START must be false in production; run the migrate command explicitly")
	}
	if cfg.SessionTTL <= 0 {
		return Config{}, fmt.Errorf("NEXORA_SESSION_TTL must be positive")
	}
	if cfg.LoginMaxFailures < 1 {
		return Config{}, fmt.Errorf("NEXORA_LOGIN_MAX_FAILURES must be at least 1 (got %d)", cfg.LoginMaxFailures)
	}
	if cfg.LoginWindow <= 0 {
		return Config{}, fmt.Errorf("NEXORA_LOGIN_WINDOW must be positive")
	}

	if cfg.Environment == EnvironmentProduction && !filepath.IsAbs(cfg.AttachmentDir) {
		return Config{}, fmt.Errorf("NEXORA_ATTACHMENT_DIR must be an explicit absolute private path in production")
	}
	return cfg, nil
}

// CookieSecure derives the session cookie Secure attribute from the
// environment. Production always sets it; there is deliberately no override
// so an insecure development cookie policy cannot leak into production.
func (c Config) CookieSecure() bool {
	return c.Environment == EnvironmentProduction
}

func validateDatabaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		// url.Error includes the original URL, including malformed credentials.
		return fmt.Errorf("NEXORA_DATABASE_URL is not a valid URL; check syntax and percent-encoding")
	}
	switch parsed.Scheme {
	case "postgres", "postgresql":
	default:
		return fmt.Errorf("NEXORA_DATABASE_URL must use the postgres or postgresql scheme")
	}
	if parsed.Opaque != "" || parsed.Fragment != "" {
		return fmt.Errorf("NEXORA_DATABASE_URL must be a hierarchical URL without a fragment")
	}
	if parsed.Host == "" {
		return fmt.Errorf("NEXORA_DATABASE_URL must include a host")
	}
	if strings.Trim(parsed.Path, "/") == "" {
		return fmt.Errorf("NEXORA_DATABASE_URL must include a database name")
	}
	if _, err := url.ParseQuery(parsed.RawQuery); err != nil {
		return fmt.Errorf("NEXORA_DATABASE_URL contains invalid query parameters; check percent-encoding")
	}
	return nil
}

// RedactedDatabaseURL exposes only the endpoint and database. All userinfo and
// query values are omitted or masked, including unknown/encoded secret keys.
func (c Config) RedactedDatabaseURL() string {
	if validateDatabaseURL(c.DatabaseURL) != nil {
		return "<invalid>"
	}
	parsed, _ := url.Parse(c.DatabaseURL)
	if parsed.User != nil {
		parsed.User = url.UserPassword("xxxxx", "xxxxx")
	}
	// Do not maintain a denylist: future driver options may also contain secrets.
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	return parsed.String()
}

func int32FromEnv(lookup func(string) string, name string, fallback int32) (int32, error) {
	raw := strings.TrimSpace(lookup(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return int32(value), nil
}

func durationFromEnv(lookup func(string) string, name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(lookup(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 10s or 1h: %w", name, err)
	}
	return value, nil
}

func boolFromEnv(lookup func(string) string, name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(lookup(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return value, nil
}

func logLevelFromEnv(lookup func(string) string, fallback slog.Level) (slog.Level, error) {
	raw := strings.ToLower(strings.TrimSpace(lookup("NEXORA_LOG_LEVEL")))
	if raw == "" {
		return fallback, nil
	}
	switch raw {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("NEXORA_LOG_LEVEL must be one of debug, info, warn, error (got %q)", raw)
	}
}
