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

func m2Reject(t *testing.T, ctx context.Context, f *testdb.Fixture, stmt string, args ...any) {
 t.Helper()
 if _, err := f.SQL.ExecContext(ctx, stmt, args...); err == nil {
  t.Fatalf("M2 should reject unsafe statement: %s", stmt)
 }
}

func TestPhase06M2FreshUpgradeEmptyRollback(t *testing.T) {
 f := testdb.New(t, os.Getenv)
 ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
 defer cancel()
 logger := slog.New(slog.NewTextHandler(io.Discard, nil))
 if err := configure(logger); err != nil { t.Fatal(err) }
 if err := goose.UpToContext(ctx, f.SQL, migrationsDir, 7); err != nil { t.Fatal(err) }
 if err := Up(ctx, f.SQL, logger); err != nil { t.Fatal(err) }
 ver, err := Version(ctx, f.SQL)
 if err != nil || ver != 8 { t.Fatalf("v8 install: %d, %v", ver, err) }
 for _, rel := range []string{
  "bonus_rounds","bonus_round_versions","bonus_round_sources","bonus_eligibility_decisions",
  "bonus_allocation_lines","bonus_adjustments","bonus_role_grants","bonus_confirmations",
  "bonus_events","bonus_member_totals",
 } {
  var exists bool
  if err := f.SQL.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", rel).Scan(&exists); err != nil || !exists {
   t.Fatalf("missing M2 object %s: %v", rel, err)
  }
 }
 if err := Down(ctx, f.SQL, 1, logger); err != nil { t.Fatal("empty v8 rollback:", err) }
 ver, err = Version(ctx, f.SQL)
 if err != nil || ver != 7 { t.Fatalf("rollback wants v7, got %d: %v", ver, err) }
 if err := Up(ctx, f.SQL, logger); err != nil { t.Fatal("reapply v8:", err) }
 ver, err = Version(ctx, f.SQL)
 if err != nil || ver != 8 { t.Fatalf("reapply wants v8, got %d: %v", ver, err) }
}

func TestPhase06M2ArchivedFundingRoundsAndFailClosed(t *testing.T) {
 f := testdb.New(t, os.Getenv)
 ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
 defer cancel()
 logger := slog.New(slog.NewTextHandler(io.Discard, nil))
 if err := Up(ctx, f.SQL, logger); err != nil { t.Fatal(err) }
 user, team, project, project2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
 round1, round2, version := uuid.New(), uuid.New(), uuid.New()
 seed := []struct{sql string; args []any}{
  {"INSERT INTO users(id,username,password_hash) VALUES($1,'m2_owner','non-login-test-hash')", []any{user}},
  {"INSERT INTO teams(id,name,created_by) VALUES($1,'M2 Team',$2)", []any{team,user}},
  {"INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner')", []any{team,user}},
  {"INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Archived competition',$3,$3)", []any{project,team,user}},
  {"INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3)", []any{team,project,user}},
  {"INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Other project',$3,$3)", []any{project2,team,user}},
  {"INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3)", []any{team,project2,user}},
  {"UPDATE projects SET status='archived',archived_at=clock_timestamp(),version=version+1 WHERE id=$1", []any{project}},
  {"INSERT INTO bonus_rounds(id,team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,$3,'school-2026','School prize','school',$4)", []any{round1,team,project,user}},
  {"INSERT INTO bonus_rounds(id,team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,$3,'college-2026','College prize','college',$4)", []any{round2,team,project,user}},
 }
 for _, x := range seed {
  if _, err := f.SQL.ExecContext(ctx,x.sql,x.args...); err != nil { t.Fatalf("seed failed: %v",err) }
 }
 var archived bool
 if err := f.SQL.QueryRowContext(ctx,"SELECT status='archived' FROM projects WHERE id=$1",project).Scan(&archived); err != nil || !archived {
  t.Fatalf("post-archive funding round changed project: %v",err)
 }
 m2Reject(t,ctx,f,"INSERT INTO bonus_rounds(team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,'school-2026','Duplicate','school',$3)",team,project,user)
 if _, err := f.SQL.ExecContext(ctx,"INSERT INTO bonus_rounds(team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,'school-2026','Different project','school',$3)",team,project2,user); err != nil {
  t.Fatalf("funding identity should be project-scoped: %v",err)
 }
 m2Reject(t,ctx,f,"INSERT INTO bonus_rounds(team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,'School-2026','Not normalized','school',$3)",team,project,user)
 _, err := f.SQL.ExecContext(ctx,"INSERT INTO bonus_round_versions(id,team_id,project_id,round_id,version,total_budget_minor,plan_unallocated_minor,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,$4,1,10000,10000,$5,'M2 draft',$6)",version,team,project,round1,user,uuid.New())
 if err != nil { t.Fatalf("draft creation failed: %v",err) }
 m2Reject(t,ctx,f,"INSERT INTO bonus_round_versions(team_id,project_id,round_id,version,total_budget_minor,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,2,1,$4,'second draft',$5)",team,project,round1,user,uuid.New())
 m2Reject(t,ctx,f,"INSERT INTO bonus_round_versions(team_id,project_id,round_id,version,total_budget_minor,currency_code,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,2,1,'USD',$4,'wrong currency',$5)",team,project,round1,user,uuid.New())
 m2Reject(t,ctx,f,"INSERT INTO bonus_round_versions(team_id,project_id,round_id,version,total_budget_minor,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,2,-1,$4,'negative money',$5)",team,project,round1,user,uuid.New())
 m2Reject(t,ctx,f,"INSERT INTO bonus_round_versions(team_id,project_id,round_id,version,total_budget_minor,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,2,900000000000001,$4,'over cap',$5)",team,project,round1,user,uuid.New())
 m2Reject(t,ctx,f,"INSERT INTO bonus_round_versions(team_id,project_id,round_id,version,status,total_budget_minor,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,2,'confirmed',1,$4,'unapproved confirmation',$5)",team,project,round1,user,uuid.New())
 m2Reject(t,ctx,f,"UPDATE bonus_round_versions SET status='pending_review' WHERE id=$1",version)
 m2Reject(t,ctx,f,"DELETE FROM bonus_rounds WHERE id=$1",round1)
 m2Reject(t,ctx,f,"INSERT INTO bonus_role_grants(team_id,project_id,round_id,subject_id,role_code,grant_mode,status,member_authorization_version,granted_by,reason,request_id) VALUES($1,$2,$3,$4,'confirmer','internal','active',1,$5,'M2 no grants','role-test')",team,project,round1,uuid.New(),user)
 m2Reject(t,ctx,f,"INSERT INTO bonus_confirmations(team_id,project_id,round_id,version_id,grant_id,confirmed_by,version_digest,reason,request_id) VALUES($1,$2,$3,$4,$5,$6,repeat('a',64),'not approved','confirm-test')",team,project,round1,version,uuid.New(),user)
 m2Reject(t,ctx,f,"INSERT INTO bonus_round_sources(team_id,project_id,round_id,version_id,task_id,case_id,case_version_id,confirmation_id,confirmation_digest,coverage_decision,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,repeat('a',64),'included','not confirmed')",team,project,round1,version,uuid.New(),uuid.New(),uuid.New(),uuid.New())
 if err := Down(ctx,f.SQL,1,logger); err == nil { t.Fatal("v8 rollback with funding evidence must be refused") }
 ver, err := Version(ctx,f.SQL)
 if err != nil || ver != 8 { t.Fatalf("failed rollback must preserve v8, got %d: %v",ver,err) }
}
