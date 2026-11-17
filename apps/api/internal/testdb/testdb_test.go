package testdb

import (
	"errors"
	"strings"
	"testing"
)

func TestTargetGuards(t *testing.T) {
	const dedicated = "postgres://test_runner@127.0.0.1:5432/nexora_test?sslmode=disable"
	tests := []struct {
		name      string
		changes   map[string]string
		wantError bool
	}{
		{"explicit disposable database", nil, false},
		{"per-run database", map[string]string{"TEST_DATABASE_URL": strings.ReplaceAll(dedicated, "/nexora_test?", "/nexora_test_run_42?")}, false},
		{"ipv6 loopback", map[string]string{"TEST_DATABASE_URL": strings.ReplaceAll(dedicated, "127.0.0.1", "[::1]")}, false},
		{"missing URL", map[string]string{"TEST_DATABASE_URL": ""}, true},
		{"no fallback", map[string]string{"TEST_DATABASE_URL": "", "NEXORA_DATABASE_URL": dedicated, "DATABASE_URL": dedicated}, true},
		{"production", map[string]string{"NEXORA_ENV": "production"}, true},
		{"development", map[string]string{"NEXORA_ENV": "development"}, true},
		{"missing acknowledgement", map[string]string{"NEXORA_TEST_DB_DISPOSABLE": ""}, true},
		{"business database", map[string]string{"TEST_DATABASE_URL": strings.ReplaceAll(dedicated, "/nexora_test?", "/nexora?")}, true},
		{"production suffix", map[string]string{"TEST_DATABASE_URL": strings.ReplaceAll(dedicated, "/nexora_test?", "/production?")}, true},
		{"production disguised as test", map[string]string{"TEST_DATABASE_URL": strings.ReplaceAll(dedicated, "/nexora_test?", "/nexora_test_prod?")}, true},
		{"remote host", map[string]string{"TEST_DATABASE_URL": strings.ReplaceAll(dedicated, "127.0.0.1", "db.production.example")}, true},
		{"host override", map[string]string{"TEST_DATABASE_URL": dedicated + "&host=production.example"}, true},
		{"database override", map[string]string{"TEST_DATABASE_URL": dedicated + "&dbname=nexora"}, true},
		{"search path override", map[string]string{"TEST_DATABASE_URL": dedicated + "&options=search_path%3Dpublic"}, true},
		{"ambient service", map[string]string{"PGSERVICE": "production"}, true},
		{"ambient options", map[string]string{"PGOPTIONS": "-c search_path=public"}, true},
		{"ambiguous TLS", map[string]string{"TEST_DATABASE_URL": strings.ReplaceAll(dedicated, "disable", "allow")}, true},
		{"fragment", map[string]string{"TEST_DATABASE_URL": dedicated + "#ignored"}, true},
		{"malformed", map[string]string{"TEST_DATABASE_URL": dedicated + "&password=%zz"}, true},
		{"invalid port", map[string]string{"TEST_DATABASE_URL": strings.ReplaceAll(dedicated, ":5432", ":invalid")}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"TEST_DATABASE_URL": dedicated, "NEXORA_ENV": "test", "NEXORA_TEST_DB_DISPOSABLE": "1"}
			for key, value := range tc.changes {
				env[key] = value
			}
			dsn, err := Target(func(key string) string { return env[key] })
			if (err != nil) != tc.wantError {
				t.Fatalf("guard error = %v, want error: %v", err, tc.wantError)
			}
			if tc.wantError && dsn != "" {
				t.Fatal("rejected configuration must not return a usable URL")
			}
			if tc.name == "no fallback" && !errors.Is(err, ErrNotConfigured) {
				t.Fatal("application URLs must not enable integration tests")
			}
		})
	}
}

func TestTargetErrorsNeverEchoCredentials(t *testing.T) {
	const marker = "SYNTHETIC_TEST_DB_SECRET"
	_, err := Target(func(key string) string {
		switch key {
		case "TEST_DATABASE_URL":
			return "postgres://test:" + marker + "%zz@localhost/nexora_test"
		case "NEXORA_ENV":
			return "test"
		case "NEXORA_TEST_DB_DISPOSABLE":
			return "1"
		default:
			return ""
		}
	})
	if err == nil || strings.Contains(err.Error(), marker) {
		t.Fatal("unsafe test URL must be rejected without echoing credentials")
	}
}
