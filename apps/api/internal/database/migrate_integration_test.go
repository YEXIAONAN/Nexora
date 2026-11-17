//go:build integration

package database

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

func TestMigrationsApplyIdempotently(t *testing.T) {
	fixture := testdb.New(t, os.Getenv)
	db := fixture.SQL

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping database: %v", SafeError(err))
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
	var schema string
	if err := db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil || !strings.HasPrefix(schema, "nexora_it_") {
		t.Fatal("migrations must run inside the fixture schema")
	}
	other := testdb.New(t, os.Getenv)
	var visible bool
	if err := other.SQL.QueryRowContext(ctx, "SELECT to_regclass('system_settings') IS NOT NULL").Scan(&visible); err != nil {
		t.Fatal("could not verify schema isolation")
	}
	if visible {
		t.Fatal("a fresh fixture can see another fixture's migrated tables")
	}
}
