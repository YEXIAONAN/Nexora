//go:build integration

package system

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryReadsBaselineState(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("NEXORA_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL or NEXORA_DATABASE_URL is required for integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	repository := NewRepository(pool)
	if err := repository.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	installationID, err := repository.InstallationID(ctx)
	if err != nil {
		t.Fatalf("installation id: %v", err)
	}
	if len(installationID) < 16 {
		t.Errorf("installation id looks invalid: %q", installationID)
	}

	version, err := repository.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version < 1 {
		t.Errorf("schema version = %d, want at least 1", version)
	}
}
