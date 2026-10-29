package config

import (
	"strings"
	"testing"
	"time"
)

func lookupFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

const testDSN = "postgres://nexora:secret@localhost:5432/nexora?sslmode=disable"

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{"NEXORA_DATABASE_URL": testDSN}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Environment != EnvironmentDevelopment {
		t.Errorf("environment = %q, want development", cfg.Environment)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("http addr = %q, want :8080", cfg.HTTPAddr)
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
	cfg := Config{DatabaseURL: testDSN}
	redacted := cfg.RedactedDatabaseURL()
	if strings.Contains(redacted, "secret") {
		t.Fatalf("redacted URL still contains the password: %s", redacted)
	}
	if !strings.Contains(redacted, "xxxxx") {
		t.Fatalf("redacted URL should contain a mask: %s", redacted)
	}
}
