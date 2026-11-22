//go:build integration

package database

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

func TestTaskProgressReportMigrationUpgradeAndRollback(t *testing.T) {
	f := testdb.New(t, os.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Pin this migration test at v9 so Down exercises progress reports, not
	// the later additive bonus foundation.
	if err := configure(logger); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, f.SQL, migrationsDir, 9); err != nil {
		t.Fatal(err)
	}
	version, err := Version(ctx, f.SQL)
	if err != nil || version != 9 {
		t.Fatalf("schema = %d, %v; want 9", version, err)
	}

	user, team, project, task := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(stmt string, args ...any) {
		t.Helper()
		if _, err := f.SQL.ExecContext(ctx, stmt, args...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	exec("INSERT INTO users(id,username,password_hash) VALUES($1,'report_owner','not-a-login-hash')", user)
	exec("INSERT INTO teams(id,name,created_by) VALUES($1,'Report Team',$2)", team, user)
	exec("INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner')", team, user)
	exec("INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Report project',$3,$3)", project, team, user)
	exec("INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3)", team, project, user)
	exec("INSERT INTO tasks(id,team_id,project_id,depth,title,owner_id,weight,created_by) VALUES($1,$2,$3,1,'Report task',$4,1,$4)", task, team, project, user)
	exec(`INSERT INTO task_progress_reports
		(team_id,project_id,task_id,actor_id,reported_progress,work_summary,evidence_links)
		VALUES($1,$2,$3,$4,40,'real execution evidence','["https://example.com/pr/1"]')`, team, project, task, user)

	if err := Down(ctx, f.SQL, 1, logger); err == nil {
		t.Fatal("rollback must refuse to drop progress evidence")
	}
	if v, _ := Version(ctx, f.SQL); v != 9 {
		t.Fatalf("blocked rollback changed schema to %d", v)
	}

	// Exercise empty rollback in a separate fixture; never delete append-only
	// business evidence or disable its guards just to make a rollback succeed.
	empty := testdb.New(t, os.Getenv)
	if err := goose.UpToContext(ctx, empty.SQL, migrationsDir, 9); err != nil {
		t.Fatal(err)
	}
	if err := Down(ctx, empty.SQL, 1, logger); err != nil {
		t.Fatal(err)
	}
	if v, _ := Version(ctx, empty.SQL); v != 8 {
		t.Fatalf("empty rollback left schema at %d", v)
	}
	if err := goose.UpToContext(ctx, empty.SQL, migrationsDir, 9); err != nil {
		t.Fatal(err)
	}
	if v, _ := Version(ctx, empty.SQL); v != 9 {
		t.Fatalf("reapply left schema at %d", v)
	}
}
