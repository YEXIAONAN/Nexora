//go:build integration

package database

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

func m2Exec(t *testing.T, ctx context.Context, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, stmt, args...); err != nil {
		t.Fatalf("M2 SQL fixture: %v", err)
	}
}

func m2Reject(t *testing.T, ctx context.Context, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, stmt, args...); err == nil {
		t.Fatalf("M2 should reject unsafe statement: %s", stmt)
	}
}

func TestPhase06M2FreshUpgradeEmptyRollback(t *testing.T) {
	f := testdb.New(t, os.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := configure(logger); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, f.SQL, migrationsDir, 7); err != nil {
		t.Fatal("v7 upgrade:", err)
	}
	if err := Up(ctx, f.SQL, logger); err != nil {
		t.Fatal("v8 upgrade:", err)
	}
	ver, err := Version(ctx, f.SQL)
	if err != nil || ver != 8 {
		t.Fatalf("v8 install: %d, %v", ver, err)
	}
	for _, name := range []string{
		"bonus_rounds",
		"bonus_round_versions",
		"bonus_round_sources",
		"bonus_eligibility_decisions",
		"bonus_allocation_lines",
		"bonus_adjustments",
		"bonus_role_grants",
		"bonus_confirmations",
		"bonus_events",
		"bonus_member_totals",
	} {
		var exists bool
		if err := f.SQL.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists); err != nil || !exists {
			t.Fatalf("missing relation %q: %v", name, err)
		}
	}
	if err := Down(ctx, f.SQL, 1, logger); err != nil {
		t.Fatal("empty v8 rollback:", err)
	}
	ver, err = Version(ctx, f.SQL)
	if err != nil || ver != 7 {
		t.Fatalf("expected v7 after empty rollback, got %d: %v", ver, err)
	}
	if err := Up(ctx, f.SQL, logger); err != nil {
		t.Fatal("reapply v8:", err)
	}
}

func TestPhase06M2ArchivedCNYRoundsFailClosed(t *testing.T) {
	f := testdb.New(t, os.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Up(ctx, f.SQL, logger); err != nil {
		t.Fatal(err)
	}
	user, team, project, otherProject := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	round, otherRound, version := uuid.New(), uuid.New(), uuid.New()

	m2Exec(t, ctx, f.SQL, "INSERT INTO users(id,username,password_hash) VALUES($1,'m2_owner','not-a-login-hash')", user)
	m2Exec(t, ctx, f.SQL, "INSERT INTO teams(id,name,created_by) VALUES($1,'M2 Team',$2)", team, user)
	m2Exec(t, ctx, f.SQL, "INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner')", team, user)
	m2Exec(t, ctx, f.SQL, "INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Archived event',$3,$3)", project, team, user)
	m2Exec(t, ctx, f.SQL, "INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3)", team, project, user)
	m2Exec(t, ctx, f.SQL, "INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Other event',$3,$3)", otherProject, team, user)
	m2Exec(t, ctx, f.SQL, "INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3)", team, otherProject, user)
	m2Exec(t, ctx, f.SQL, "UPDATE projects SET status='archived',archived_at=clock_timestamp(),version=version+1 WHERE id=$1", project)

	insertRound := "INSERT INTO bonus_rounds(id,team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,$3,$4,'Award','source',$5)"
	m2Exec(t, ctx, f.SQL, insertRound, round, team, project, "school-2026", user)
	m2Exec(t, ctx, f.SQL, insertRound, otherRound, team, project, "college-2026", user)
	m2Reject(t, ctx, f.SQL, insertRound, uuid.New(), team, project, "school-2026", user)
	m2Reject(t, ctx, f.SQL, insertRound, uuid.New(), team, project, "School-2026", user)
	m2Exec(t, ctx, f.SQL, insertRound, uuid.New(), team, otherProject, "school-2026", user)

	var archived bool
	if err := f.SQL.QueryRowContext(ctx, "SELECT status='archived' FROM projects WHERE id=$1", project).Scan(&archived); err != nil || !archived {
		t.Fatalf("post-archive bonus changed original project: %v", err)
	}
	insertVersion := "INSERT INTO bonus_round_versions(id,team_id,project_id,round_id,version,status,total_budget_minor,plan_unallocated_minor,currency_code,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'M2 draft',$11)"
	m2Exec(t, ctx, f.SQL, insertVersion, version, team, project, round, 1, "draft", int64(10000), int64(10000), "CNY", user, uuid.New())
	m2Reject(t, ctx, f.SQL, insertVersion, uuid.New(), team, project, round, 2, "draft", int64(100), int64(100), "CNY", user, uuid.New())
	m2Reject(t, ctx, f.SQL, insertVersion, uuid.New(), team, project, otherRound, 1, "draft", int64(100), int64(100), "USD", user, uuid.New())
	m2Reject(t, ctx, f.SQL, insertVersion, uuid.New(), team, project, otherRound, 1, "draft", int64(-1), int64(0), "CNY", user, uuid.New())
	m2Reject(t, ctx, f.SQL, insertVersion, uuid.New(), team, project, otherRound, 1, "draft", int64(900000000000001), int64(0), "CNY", user, uuid.New())
	m2Reject(t, ctx, f.SQL, insertVersion, uuid.New(), team, project, otherRound, 1, "confirmed", int64(100), int64(0), "CNY", user, uuid.New())
	m2Reject(t, ctx, f.SQL, "UPDATE bonus_round_versions SET status='pending_review' WHERE id=$1", version)
	m2Reject(t, ctx, f.SQL, "DELETE FROM bonus_rounds WHERE id=$1", round)

	// The internal member epoch is mandatory even for proposed grants.
	// PostgreSQL CHECK accepts NULL unless explicitly guarded (SQL three-valued logic).
	reviewer := uuid.New()
	m2Exec(t, ctx, f.SQL, "INSERT INTO users(id,username,password_hash) VALUES($1,'m2_reviewer','not-a-login-hash')", reviewer)
	m2Reject(t, ctx, f.SQL, "INSERT INTO bonus_role_grants(team_id,project_id,round_id,subject_id,role_code,grant_mode,status,member_authorization_version,granted_by,reason,request_id) VALUES($1,$2,$3,$4,'confirmer','internal','proposed',NULL,$5,'epoch required','grant-null-epoch')", team, project, round, reviewer, user)
	m2Reject(t, ctx, f.SQL, "INSERT INTO bonus_role_grants(team_id,project_id,round_id,subject_id,role_code,grant_mode,status,member_authorization_version,granted_by,reason,request_id) VALUES($1,$2,$3,$4,'confirmer','internal','active',1,$5,'not approved','grant-1')", team, project, round, uuid.New(), user)
	m2Reject(t, ctx, f.SQL, "INSERT INTO bonus_confirmations(team_id,project_id,round_id,version_id,grant_id,confirmed_by,version_digest,reason,request_id) VALUES($1,$2,$3,$4,$5,$6,repeat('a',64),'not approved','confirm-1')", team, project, round, version, uuid.New(), user)
	m2Reject(t, ctx, f.SQL, "INSERT INTO bonus_round_sources(team_id,project_id,round_id,version_id,task_id,case_id,case_version_id,confirmation_id,confirmation_digest,coverage_decision,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,repeat('a',64),'included','fake evidence')", team, project, round, version, uuid.New(), uuid.New(), uuid.New(), uuid.New())

	if err := Down(ctx, f.SQL, 1, logger); err == nil {
		t.Fatal("rollback with funding evidence must be rejected")
	}
	ver, err := Version(ctx, f.SQL)
	if err != nil || ver != 8 {
		t.Fatalf("blocked rollback changed version: %d, %v", ver, err)
	}
}
