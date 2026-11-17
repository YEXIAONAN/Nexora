package config

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func lookupFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

const testDSN = "postgres://nexora@localhost:5432/nexora?sslmode=disable"

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{"NEXORA_DATABASE_URL": testDSN}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Environment != EnvironmentDevelopment {
		t.Errorf("environment = %q, want development", cfg.Environment)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("http addr = %q, want 127.0.0.1:8080", cfg.HTTPAddr)
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("max conns = %d, want 10", cfg.DBMaxConns)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("shutdown timeout = %s, want 15s", cfg.ShutdownTimeout)
	}
	if cfg.MigrateOnStart {
		t.Error("migrate on start should default to false")
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{
		"NEXORA_ENV":                "test",
		"NEXORA_HTTP_ADDR":          "127.0.0.1:9090",
		"NEXORA_DATABASE_URL":       testDSN,
		"NEXORA_DB_MAX_CONNS":       "25",
		"NEXORA_DB_MIN_CONNS":       "5",
		"NEXORA_DB_CONNECT_TIMEOUT": "3s",
		"NEXORA_SHUTDOWN_TIMEOUT":   "20s",
		"NEXORA_MIGRATE_ON_START":   "true",
		"NEXORA_LOG_LEVEL":          "debug",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Environment != EnvironmentTest || cfg.HTTPAddr != "127.0.0.1:9090" {
		t.Errorf("unexpected environment/addr: %+v", cfg)
	}
	if cfg.DBMaxConns != 25 || cfg.DBMinConns != 5 {
		t.Errorf("unexpected pool bounds: %d/%d", cfg.DBMaxConns, cfg.DBMinConns)
	}
	if cfg.DBConnectTimeout != 3*time.Second || cfg.ShutdownTimeout != 20*time.Second {
		t.Errorf("unexpected durations: %+v", cfg)
	}
	if !cfg.MigrateOnStart {
		t.Error("migrate on start should be true")
	}
}

func TestSessionSecurityDefaults(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{"NEXORA_DATABASE_URL": testDSN}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Errorf("session ttl = %s, want 12h", cfg.SessionTTL)
	}
	if cfg.LoginMaxFailures != 10 || cfg.LoginWindow != 15*time.Minute {
		t.Errorf("login limiter defaults = %d/%s, want 10/15m", cfg.LoginMaxFailures, cfg.LoginWindow)
	}
	if cfg.CookieSecure() {
		t.Error("development cookies must not set Secure over plain HTTP")
	}
}

func TestSessionSecurityOverrides(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{
		"NEXORA_DATABASE_URL":       testDSN,
		"NEXORA_SESSION_TTL":        "1h",
		"NEXORA_LOGIN_MAX_FAILURES": "3",
		"NEXORA_LOGIN_WINDOW":       "5m",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.SessionTTL != time.Hour || cfg.LoginMaxFailures != 3 || cfg.LoginWindow != 5*time.Minute {
		t.Errorf("unexpected session security overrides: %+v", cfg)
	}
}

func TestCookieSecureIsEnvironmentDerived(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{
		{"development", false},
		{"test", false},
		{"production", true},
	} {
		cfg, err := load(lookupFrom(map[string]string{"NEXORA_DATABASE_URL": testDSN, "NEXORA_ENV": tc.env, "NEXORA_ATTACHMENT_DIR": "/private/nexora/attachments"}))
		if err != nil {
			t.Fatalf("load %s: %v", tc.env, err)
		}
		if cfg.CookieSecure() != tc.want {
			t.Errorf("%s: CookieSecure = %v, want %v", tc.env, cfg.CookieSecure(), tc.want)
		}
	}
}

func TestLoadRejectsInvalidSessionSecurity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		env    map[string]string
		errSub string
	}{
		{"zero session ttl", map[string]string{"NEXORA_DATABASE_URL": testDSN, "NEXORA_SESSION_TTL": "0s"}, "NEXORA_SESSION_TTL"},
		{"negative session ttl", map[string]string{"NEXORA_DATABASE_URL": testDSN, "NEXORA_SESSION_TTL": "-1h"}, "NEXORA_SESSION_TTL"},
		{"zero failure threshold", map[string]string{"NEXORA_DATABASE_URL": testDSN, "NEXORA_LOGIN_MAX_FAILURES": "0"}, "NEXORA_LOGIN_MAX_FAILURES"},
		{"non numeric threshold", map[string]string{"NEXORA_DATABASE_URL": testDSN, "NEXORA_LOGIN_MAX_FAILURES": "many"}, "NEXORA_LOGIN_MAX_FAILURES"},
		{"zero window", map[string]string{"NEXORA_DATABASE_URL": testDSN, "NEXORA_LOGIN_WINDOW": "0s"}, "NEXORA_LOGIN_WINDOW"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(lookupFrom(tc.env))
			if err == nil || !strings.Contains(err.Error(), tc.errSub) {
				t.Fatalf("expected error containing %q, got %v", tc.errSub, err)
			}
		})
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		errSub string
	}{
		{
			name:   "missing database url",
			env:    map[string]string{},
			errSub: "NEXORA_DATABASE_URL is required",
		},
		{
			name:   "wrong scheme",
			env:    map[string]string{"NEXORA_DATABASE_URL": "mysql://localhost:3306/nexora"},
			errSub: "postgres or postgresql",
		},
		{
			name:   "missing database name",
			env:    map[string]string{"NEXORA_DATABASE_URL": "postgres://localhost:5432"},
			errSub: "database name",
		},
		{
			name: "unknown environment",
			env: map[string]string{
				"NEXORA_ENV":          "staging",
				"NEXORA_DATABASE_URL": testDSN,
			},
			errSub: "NEXORA_ENV",
		},
		{
			name: "invalid pool size",
			env: map[string]string{
				"NEXORA_DATABASE_URL": testDSN,
				"NEXORA_DB_MAX_CONNS": "12abc",
			},
			errSub: "NEXORA_DB_MAX_CONNS",
		},
		{
			name: "min conns above max",
			env: map[string]string{
				"NEXORA_DATABASE_URL": testDSN,
				"NEXORA_DB_MAX_CONNS": "5",
				"NEXORA_DB_MIN_CONNS": "6",
			},
			errSub: "between 0 and NEXORA_DB_MAX_CONNS",
		},
		{
			name: "production auto migration",
			env: map[string]string{
				"NEXORA_ENV":              "production",
				"NEXORA_DATABASE_URL":     testDSN,
				"NEXORA_MIGRATE_ON_START": "true",
			},
			errSub: "must be false in production",
		},
		{
			name: "unknown log level",
			env: map[string]string{
				"NEXORA_DATABASE_URL": testDSN,
				"NEXORA_LOG_LEVEL":    "verbose",
			},
			errSub: "NEXORA_LOG_LEVEL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(lookupFrom(tc.env))
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.errSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errSub)
			}
		})
	}
}

func TestRedactedDatabaseURL(t *testing.T) {
	const marker = "synthetic-password+/中文"
	withPassword, _ := url.Parse(testDSN)
	withPassword.User = url.UserPassword("synthetic-user", marker)
	tests := []struct {
		name  string
		dsn   string
		valid bool
	}{
		{"normal", testDSN, true},
		{"encoded userinfo", withPassword.String(), true},
		{"query secrets", testDSN + "&password=" + url.QueryEscape(marker) + "&sslpassword=" + url.QueryEscape(marker), true},
		{"encoded secret key", testDSN + "&pass%77ord=" + url.QueryEscape(marker), true},
		{"unknown secret option", testDSN + "&custom_token=" + url.QueryEscape(marker), true},
		{"malformed userinfo", "postgres://user:" + marker + "%zz@localhost/db", false},
		{"malformed query", testDSN + "&password=%zz", false},
		{"fragment", testDSN + "#" + marker, false},
		{"wrong scheme", "mysql://localhost/db", false},
		{"missing host", "postgres:///db", false},
		{"missing database", "postgres://localhost", false},
		{"empty", "", false},
		{"whitespace", "  ", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			redacted := (Config{DatabaseURL: tc.dsn}).RedactedDatabaseURL()
			for _, secret := range []string{marker, url.QueryEscape(marker), url.PathEscape(marker), "synthetic-user", "custom_token", "pass%77ord"} {
				if strings.Contains(redacted, secret) {
					t.Fatal("redacted endpoint contains synthetic sensitive information")
				}
			}
			if !tc.valid {
				if redacted != "<invalid>" {
					t.Fatal("invalid URL must not be rendered")
				}
				return
			}
			parsed, err := url.Parse(redacted)
			if err != nil || parsed.Host != "localhost:5432" || parsed.Path != "/nexora" || parsed.RawQuery != "" {
				t.Fatal("redaction must preserve the endpoint and remove all query values")
			}
			if tc.name == "encoded userinfo" && !strings.Contains(redacted, "xxxxx") {
				t.Fatal("userinfo must be masked")
			}
		})
	}
}

func TestInvalidDatabaseURLErrorsDoNotEchoInput(t *testing.T) {
	const marker = "SYNTHETIC_CONFIG_SECRET"
	for _, dsn := range []string{
		"postgres://user:" + marker + "%zz@localhost/db",
		"postgres://localhost/db?password=" + marker + "%zz",
		marker + "://localhost/db",
		"postgres://localhost/db#" + marker,
	} {
		_, err := load(lookupFrom(map[string]string{"NEXORA_DATABASE_URL": dsn}))
		if err == nil {
			t.Fatal("invalid connection URL should be rejected")
		}
		if strings.Contains(err.Error(), marker) || strings.Contains(err.Error(), dsn) {
			t.Fatal("configuration error echoed synthetic credentials")
		}
		if !strings.Contains(err.Error(), "NEXORA_DATABASE_URL") {
			t.Fatal("configuration error must identify the failing setting")
		}
	}
}

func TestProductionPrivateStoragePath(t *testing.T) {
	for _, path := range []string{"", "relative/path"} {
		if _, e := load(lookupFrom(map[string]string{"NEXORA_DATABASE_URL": testDSN, "NEXORA_ENV": "production", "NEXORA_ATTACHMENT_DIR": path})); e == nil {
			t.Fatal("production accepted non-absolute private storage path")
		}
	}
}
