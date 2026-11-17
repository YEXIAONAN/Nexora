//go:build integration

package system

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

func TestRepositoryReadsBaselineState(t *testing.T) {
	fixture := testdb.New(t, os.Getenv)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := database.Up(ctx, fixture.SQL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("prepare isolated baseline: %v", err)
	}

	repository := NewRepository(fixture.Pool)
	if err := repository.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", database.SafeError(err))
	}

	installationID, err := repository.InstallationID(ctx)
	if err != nil {
		t.Fatalf("installation id: %v", database.SafeError(err))
	}
	if len(installationID) < 16 {
		t.Errorf("installation id looks invalid: %q", installationID)
	}

	version, err := repository.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("schema version: %v", database.SafeError(err))
	}
	if version < 1 {
		t.Errorf("schema version = %d, want at least 1", version)
	}
}
