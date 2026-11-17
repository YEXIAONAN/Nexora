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
	"github.com/pressly/goose/v3"
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

func TestPhase03FreshUpgradeAndRollback(t *testing.T) {
	f := testdb.New(t, os.Getenv)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	check := func(want int64) {
		t.Helper()
		got, e := Version(ctx, f.SQL)
		if e != nil || got != want {
			t.Fatalf("schema %d want %d: %v", got, want, e)
		}
	}
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 5); e != nil {
		t.Fatal(e)
	}
	check(5)
	if e := Down(ctx, f.SQL, 2, logger); e != nil {
		t.Fatal(e)
	}
	check(3)
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 5); e != nil {
		t.Fatal(e)
	}
	check(5)
	if e := Down(ctx, f.SQL, 5, logger); e != nil {
		t.Fatal(e)
	}
	check(0)
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 5); e != nil {
		t.Fatal(e)
	}
	check(5)
}

// v5 upgrade, new installation, full empty rollback and reapply use only isolated fixtures.
func TestPhase04FreshUpgradeAndRollback(t *testing.T) {
	f := testdb.New(t, os.Getenv)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if e := configure(logger); e != nil {
		t.Fatal(e)
	}
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 5); e != nil {
		t.Fatal(e)
	}
	if e := Up(ctx, f.SQL, logger); e != nil {
		t.Fatal(e)
	}
	check := func(want int64) {
		t.Helper()
		v, e := Version(ctx, f.SQL)
		if e != nil || v != want {
			t.Fatalf("schema %d want %d: %v", v, want, e)
		}
	}
	check(6)
	if e := Down(ctx, f.SQL, 1, logger); e != nil {
		t.Fatal(e)
	}
	check(5)
	if e := Up(ctx, f.SQL, logger); e != nil {
		t.Fatal(e)
	}
	check(6)
	if e := Down(ctx, f.SQL, 6, logger); e != nil {
		t.Fatal(e)
	}
	check(0)
	if e := Up(ctx, f.SQL, logger); e != nil {
		t.Fatal(e)
	}
	check(6)
}
