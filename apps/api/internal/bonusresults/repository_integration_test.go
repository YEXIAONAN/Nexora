//go:build integration

package bonusresults_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/bonusresults"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

func TestM3MineFiltersDraftsAndRequiresActiveSessionMembership(t *testing.T) {
	f := testdb.New(t, os.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := database.Up(ctx, f.SQL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.SQL.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	owner, member, root := uuid.New(), uuid.New(), uuid.New()
	team, project, round, session, expiredSession, rootSession := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO users(id,username,password_hash) VALUES($1,'m3_owner','not-a-login-hash'),($2,'m3_member','not-a-login-hash')", owner, member)
	exec("INSERT INTO users(id,username,password_hash,system_root) VALUES($1,'m3_root','not-a-login-hash',true)", root)
	exec("INSERT INTO teams(id,name,created_by) VALUES($1,'M3 read-only team',$2)", team, owner)
	exec("INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'member')", team, owner, member)
	exec("INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Archived source project',$3,$3)", project, team, owner)
	exec("INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3)", team, project, member)
	exec("UPDATE projects SET status='archived',archived_at=clock_timestamp(),version=version+1 WHERE id=$1", project)
	exec("INSERT INTO bonus_rounds(id,team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,$3,'school-2026','School prize','school',$4)", round, team, project, owner)
	exec("INSERT INTO bonus_round_versions(team_id,project_id,round_id,version,total_budget_minor,plan_unallocated_minor,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,1,100,100,$4,'draft only',$5)", team, project, round, owner, uuid.New())
	exec("INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES($1,$2,repeat('a',64),now()+interval '1 hour')", session, member)
	exec("INSERT INTO sessions(id,user_id,token_hash,created_at,expires_at) VALUES($1,$2,repeat('b',64),now()-interval '2 hours',now()-interval '1 hour')", expiredSession, member)
	exec("INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES($1,$2,repeat('c',64),now()+interval '1 hour')", rootSession, root)

	svc := bonusresults.NewService(bonusresults.NewRepository(f.Pool))
	subject := reqctx.Identity{UserID: member, SessionID: session}
	results, err := svc.Mine(ctx, subject, team)
	if err != nil {
		t.Fatal(err)
	}
	if results.Items == nil || len(results.Items) != 0 || results.HasMore {
		t.Fatalf("a draft must not appear as a formal personal bonus: %+v", results)
	}
	assertNotFound := func(id reqctx.Identity) {
		t.Helper()
		_, err := svc.Mine(ctx, id, team)
		api, ok := err.(*httpx.APIError)
		if !ok || api.Status != http.StatusNotFound {
			t.Fatalf("invalid access was not rejected as 404: %v", err)
		}
	}
	assertNotFound(reqctx.Identity{UserID: member, SessionID: expiredSession})
	assertNotFound(reqctx.Identity{UserID: root, SessionID: rootSession, SystemRoot: true})
	exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=$1", session)
	assertNotFound(subject)
}
