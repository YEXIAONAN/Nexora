// Package testdb provides guarded, isolated PostgreSQL integration fixtures.
// It is used only by tests; it never falls back to application configuration.
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

var (
	ErrNotConfigured = errors.New("TEST_DATABASE_URL is required; database integration tests skipped")
	testName         = regexp.MustCompile(`^nexora_test(?:_[a-z0-9_]+)?$`)
	productionName   = regexp.MustCompile(`(?:^|_)(?:prod|production|live)(?:_|$)`)
)

// Target requires an explicit disposable-test acknowledgement, test mode, a
// loopback endpoint and a dedicated database name before any connection occurs.
func Target(lookup func(string) string) (string, error) {
	dsn := strings.TrimSpace(lookup("TEST_DATABASE_URL"))
	if dsn == "" {
		return "", ErrNotConfigured
	}
	if lookup("NEXORA_ENV") != "test" {
		return "", errors.New("integration tests require NEXORA_ENV=test")
	}
	if lookup("NEXORA_TEST_DB_DISPOSABLE") != "1" {
		return "", errors.New("integration tests require NEXORA_TEST_DB_DISPOSABLE=1 for an independent disposable database")
	}
	for _, key := range []string{"PGSERVICE", "PGSERVICEFILE", "PGHOST", "PGHOSTADDR", "PGPORT", "PGDATABASE", "PGUSER", "PGOPTIONS", "PGTARGETSESSIONATTRS"} {
		if lookup(key) != "" {
			return "", errors.New("integration tests refuse ambient connection override: " + key)
		}
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Opaque != "" || u.Fragment != "" {
		return "", errors.New("TEST_DATABASE_URL must be a valid PostgreSQL URL without a fragment")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", errors.New("TEST_DATABASE_URL must use a loopback host; remote targets are refused")
	}
	databaseName := strings.TrimPrefix(u.Path, "/")
	if u.User == nil || u.User.Username() == "" || !testName.MatchString(databaseName) || productionName.MatchString(databaseName) {
		return "", errors.New("TEST_DATABASE_URL requires an explicit user and database named nexora_test or nexora_test_<suffix>")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("TEST_DATABASE_URL contains invalid query parameters")
	}
	// URL options must not redirect the host/database or inject a search_path.
	for key, values := range query {
		if key != "sslmode" || len(values) != 1 {
			return "", errors.New("TEST_DATABASE_URL only permits one sslmode query option")
		}
		switch values[0] {
		case "disable", "require", "verify-ca", "verify-full":
		default:
			return "", errors.New("TEST_DATABASE_URL contains an unsupported sslmode")
		}
	}
	if _, err := pgx.ParseConfig(dsn); err != nil {
		return "", errors.New("TEST_DATABASE_URL has invalid connection options")
	}
	return dsn, nil
}

type Fixture struct {
	SQL  *sql.DB
	Pool *pgxpool.Pool
}

// New confines migrations and queries to a fresh, randomly named schema.
// Cleanup drops only that generated schema, including when a test fails.
func New(t testing.TB, lookup func(string) string) *Fixture {
	t.Helper()
	dsn, err := Target(lookup)
	if errors.Is(err, ErrNotConfigured) {
		t.Skip(ErrNotConfigured.Error())
	}
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("test database configuration could not be parsed")
	}
	conn.ConnectTimeout = 5 * time.Second
	if conn.Host == "localhost" {
		// Avoid resolving the allowed localhost name to a non-loopback address.
		conn.LookupFunc = func(context.Context, string) ([]string, error) { return []string{"127.0.0.1", "::1"}, nil }
	}
	admin := stdlib.OpenDB(*conn)
	admin.SetMaxOpenConns(1)
	fixture := &Fixture{}
	schema := "nexora_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	created := false
	t.Cleanup(func() {
		if fixture.SQL != nil {
			fixture.SQL.Close()
		}
		if fixture.Pool != nil {
			fixture.Pool.Close()
		}
		if created {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := admin.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
				t.Errorf("failed to clean up generated integration-test schema %s; database details withheld", schema)
			}
		}
		admin.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var actual string
	if err := admin.QueryRowContext(ctx, "SELECT current_database()").Scan(&actual); err != nil {
		t.Fatal("cannot connect to the explicitly configured test database; connection details withheld")
	}
	if actual != conn.Database || !testName.MatchString(actual) {
		t.Fatal("connected database does not match the dedicated test target")
	}
	// Also attempt cleanup if the CREATE response is lost after PostgreSQL commits.
	created = true
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal("cannot create an isolated integration-test schema")
	}
	isolated := conn.Copy()
	isolated.RuntimeParams["search_path"] = schema
	isolated.RuntimeParams["timezone"] = "UTC"
	fixture.SQL = stdlib.OpenDB(*isolated)
	fixture.SQL.SetMaxOpenConns(2)
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("cannot configure the integration-test pool")
	}
	poolConfig.ConnConfig = isolated.Copy()
	poolConfig.MaxConns = 2
	fixture.Pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal("cannot create the integration-test pool")
	}
	return fixture
}
