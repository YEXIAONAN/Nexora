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
	if version != MaximumSupportedSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, MaximumSupportedSchemaVersion)
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
	if err := configure(logger); err != nil {
		t.Fatal(err)
	}
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
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 6); e != nil {
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
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 6); e != nil {
		t.Fatal(e)
	}
	check(6)
	if e := Down(ctx, f.SQL, 6, logger); e != nil {
		t.Fatal(e)
	}
	check(0)
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 6); e != nil {
		t.Fatal(e)
	}
	check(6)
}

func TestPhase04UpgradePreservesPhase03History(t *testing.T) {
	f := testdb.New(t, os.Getenv)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if e := configure(logger); e != nil {
		t.Fatal(e)
	}
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 5); e != nil {
		t.Fatal(e)
	}
	// Seed only an old v5 execution/history/snapshot, before any Phase04 tables
	// exist. This is migration evidence, not an API acceptance shortcut.
	_, e := f.SQL.ExecContext(ctx, `DO $$ DECLARE u uuid:=gen_random_uuid(); tm uuid:=gen_random_uuid(); p uuid:=gen_random_uuid(); t uuid:=gen_random_uuid(); BEGIN
 INSERT INTO users(id,username,password_hash) VALUES(u,'legacy_owner','migration-fixture-not-a-login-hash');
 INSERT INTO teams(id,name,created_by) VALUES(tm,'Legacy Team',u);
 INSERT INTO team_members(team_id,user_id,role) VALUES(tm,u,'owner');
 INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES(p,tm,'Legacy project',u,u);
 INSERT INTO project_members(team_id,project_id,user_id) VALUES(tm,p,u);
 INSERT INTO tasks(id,team_id,project_id,depth,title,owner_id,weight,created_by) VALUES(t,tm,p,1,'Legacy execution',u,3,u);
 UPDATE tasks SET status='in_progress',actual_progress=60,version=2 WHERE id=t;
 INSERT INTO task_progress_history(team_id,project_id,task_id,actor_id,event_type,old_status,new_status,old_progress,new_progress,old_weight,new_weight,before_project_bps,after_project_bps,request_id) VALUES(tm,p,t,u,'progress','in_progress','in_progress',0,60,3,3,0,6000,'legacy-progress');
 INSERT INTO project_progress_snapshots(team_id,project_id,task_id,actor_id,event_type,progress_bps,weighted_sum,total_weight,leaf_count,request_id) VALUES(tm,p,t,u,'progress',6000,18000,3,1,'legacy-progress');
 INSERT INTO audit_events(actor_id,team_id,target_id,action,request_id) VALUES(u,tm,t,'task.progress','legacy-progress');
 END $$`)
	if e != nil {
		t.Fatal(e)
	}
	fingerprint := func() string {
		t.Helper()
		var v string
		e := f.SQL.QueryRowContext(ctx, `SELECT jsonb_build_object('task',(SELECT to_jsonb(t)-ARRAY['current_submission_version_id','accepted_review_id'] FROM tasks t),'history',(SELECT jsonb_agg(to_jsonb(h) ORDER BY h.id) FROM task_progress_history h),'snapshots',(SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM project_progress_snapshots s),'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM audit_events a))::text`).Scan(&v)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	before := fingerprint()
	if e = goose.UpToContext(ctx, f.SQL, migrationsDir, 6); e != nil {
		t.Fatal(e)
	}
	if fingerprint() != before {
		t.Fatal("v6 upgrade changed legacy execution or immutable history")
	}
	if _, e = f.SQL.ExecContext(ctx, `UPDATE tasks SET status='completed',version=version+1`); e == nil {
		t.Fatal("legacy task completed without current approval")
	}
	if e = Down(ctx, f.SQL, 1, logger); e != nil {
		t.Fatal(e)
	}
	if fingerprint() != before {
		t.Fatal("empty Phase04 rollback changed legacy history")
	}
}

func TestPhase05SchemaInstallRollback(t *testing.T) {
	f := testdb.New(t, os.Getenv)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if e := configure(logger); e != nil {
		t.Fatal(e)
	}
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 6); e != nil {
		t.Fatal(e)
	}
	// Pin this historical test at v7 even when newer migrations exist.
	if e := goose.UpToContext(ctx, f.SQL, migrationsDir, 7); e != nil {
		t.Fatal(e)
	}
	v, e := Version(ctx, f.SQL)
	if e != nil || v != 7 {
		t.Fatal(v, e)
	}
	if e = Down(ctx, f.SQL, 1, logger); e != nil {
		t.Fatal(e)
	}
	v, e = Version(ctx, f.SQL)
	if e != nil || v != 6 {
		t.Fatal(v, e)
	}
	if e = Down(ctx, f.SQL, 6, logger); e != nil {
		t.Fatal(e)
	}
	if e = goose.UpToContext(ctx, f.SQL, migrationsDir, 7); e != nil {
		t.Fatal(e)
	}
	v, e = Version(ctx, f.SQL)
	if e != nil || v != 7 {
		t.Fatal(v, e)
	}
}
