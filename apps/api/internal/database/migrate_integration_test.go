//go:build integration

package database

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"
)

func integrationDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("NEXORA_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL or NEXORA_DATABASE_URL is required for integration tests")
	}
	return dsn
}

func TestMigrationsApplyIdempotently(t *testing.T) {
	dsn := integrationDSN(t)
	db, err := OpenSQL(dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Up(ctx, db, logger); err != nil {
		t.Fatalf("first migration run: %v", err)
	}
	version, err := Version(ctx, db)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version < 1 {
		t.Fatalf("schema version = %d, want at least 1", version)
	}

	if err := Up(ctx, db, logger); err != nil {
		t.Fatalf("second migration run should be a no-op: %v", err)
	}
}
