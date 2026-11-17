package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
)

func TestSafeErrorPreservesClassificationWithoutCredentials(t *testing.T) {
	const marker = "SYNTHETIC_DRIVER_SECRET"
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"parse", pgconn.NewParseConfigError(marker, marker, errors.New(marker)), "invalid database connection configuration"},
		{"postgres", &pgconn.PgError{Code: "28P01", Message: marker, Detail: marker}, "SQLSTATE 28P01"},
		{"invalid sqlstate", &pgconn.PgError{Code: marker, Message: marker}, "database operation failed"},
		{"dns", &net.DNSError{Err: marker, Name: marker}, "hostname lookup failed"},
		{"refused", fmt.Errorf("%s: %w", marker, syscall.ECONNREFUSED), "connection refused"},
		{"deadline", fmt.Errorf("%s: %w", marker, context.DeadlineExceeded), "timed out"},
		{"cancel", context.Canceled, "canceled"},
		{"unknown", errors.New(marker), "database operation failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			safe := SafeError(tc.err)
			if strings.Contains(safe.Error(), marker) || !strings.Contains(safe.Error(), tc.want) {
				t.Fatal("error must preserve only a safe diagnostic category")
			}
			if !errors.Is(safe, tc.err) {
				t.Fatal("original cause must remain available to errors.Is/As")
			}
		})
	}
	if SafeError(nil) != nil {
		t.Fatal("nil error should stay nil")
	}
}

func TestConnectionParsingErrorsAreSafeWithoutConnecting(t *testing.T) {
	const marker = "SYNTHETIC_PARSE_SECRET"
	dsn := "postgres://audit@127.0.0.1/audit?password=" + marker + "&port=invalid"
	_, err := Open(context.Background(), config.Config{DatabaseURL: dsn})
	if err == nil || strings.Contains(err.Error(), marker) || !strings.Contains(err.Error(), "parse database configuration") {
		t.Fatal("pool parsing must fail safely before opening a connection")
	}
	db, err := OpenSQL(dsn)
	if db != nil {
		db.Close()
	}
	if err == nil || strings.Contains(err.Error(), marker) || !strings.Contains(err.Error(), "open database") {
		t.Fatal("migration connection parsing must fail without exposing input")
	}
}
