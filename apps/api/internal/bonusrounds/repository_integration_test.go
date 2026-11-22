//go:build integration

package bonusrounds_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/bonusrounds"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

type roundFixture struct {
	t       *testing.T
	ctx     context.Context
	db      *testdb.Fixture
	team    uuid.UUID
	project uuid.UUID
	owner   uuid.UUID
	member  uuid.UUID
	session uuid.UUID
	rounds  []uuid.UUID
}

func newRoundFixture(t *testing.T) *roundFixture {
	t.Helper()
	f := &roundFixture{t: t, db: testdb.New(t, os.Getenv)}
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	if err := database.Up(f.ctx, f.db.SQL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	f.team, f.project, f.owner, f.member, f.session = uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.db.Pool.Exec(f.ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO users(id,username,password_hash) VALUES($1,'round_owner','not-a-login-hash'),($2,'round_member','not-a-login-hash')", f.owner, f.member)
	exec("INSERT INTO teams(id,name,created_by) VALUES($1,'Round team',$2)", f.team, f.owner)
	exec("INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'member')", f.team, f.owner, f.member)
	exec("INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Round project',$3,$3)", f.project, f.team, f.owner)
	exec("INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3)", f.team, f.project, f.member)
	exec("UPDATE projects SET status='archived',archived_at=clock_timestamp(),version=version+1 WHERE id=$1", f.project)
	exec("INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES($1,$2,repeat('e',64),now()+interval '1 hour')", f.session, f.member)
	f.rounds = []uuid.UUID{
		f.insertDraftRound("school-2026", "School prize", "school"),
		f.insertDraftRound("college-2026", "College prize", "college"),
	}
	return f
}

// The M2 triggers intentionally reject UPDATE on bonus_rounds, so the round
// pointer and its draft version are created in one deferred-FK transaction.
func (f *roundFixture) insertDraftRound(key, label, source string) uuid.UUID {
	f.t.Helper()
	round, version := uuid.New(), uuid.New()
	tx, err := f.db.Pool.Begin(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	defer tx.Rollback(f.ctx) //nolint:errcheck // rollback after commit is harmless
	if _, err := tx.Exec(f.ctx, `INSERT INTO bonus_rounds(id,team_id,project_id,funding_key,funding_label,funding_source,created_by,latest_version_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, round, f.team, f.project, key, label, source, f.owner, version); err != nil {
		f.t.Fatal(err)
	}
	if _, err := tx.Exec(f.ctx, `INSERT INTO bonus_round_versions(id,team_id,project_id,round_id,version,total_budget_minor,plan_unallocated_minor,proposed_by,rationale,client_request_id)
		VALUES($1,$2,$3,$4,1,100,100,$5,'draft metadata only',$6)`, version, f.team, f.project, round, f.owner, uuid.New()); err != nil {
		f.t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	return round
}

func (f *roundFixture) identity() reqctx.Identity {
	return reqctx.Identity{UserID: f.member, SessionID: f.session}
}

func requireNotFound(t *testing.T, err error, label string) {
	t.Helper()
	var api *httpx.APIError
	if !errors.As(err, &api) || api.Status != http.StatusNotFound {
		t.Fatalf("%s: error = %v; want HTTP 404", label, err)
	}
}

func TestListAndGetRoundMetadataInArchivedProject(t *testing.T) {
	f := newRoundFixture(t)
	svc := bonusrounds.NewService(bonusrounds.NewRepository(f.db.Pool))
	page, err := svc.List(f.ctx, f.identity(), f.team, f.project)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.HasMore {
		t.Fatalf("archived project rounds = %d, want 2", len(page.Items))
	}
	seen := map[string]bool{}
	for _, item := range page.Items {
		seen[item.FundingLabel] = true
		if item.Status != bonusrounds.StatusDraft || item.AmountsVisible {
			t.Fatalf("unexpected round summary: %+v", item)
		}
		if item.LatestVersion == nil || item.LatestVersion.Version != 1 || item.LatestVersion.Status != "draft" {
			t.Fatalf("latest draft version missing: %+v", item.LatestVersion)
		}
		if item.EffectiveConfirmedVersion != nil {
			t.Fatal("draft-only round exposed a formal result")
		}
	}
	if !seen["School prize"] || !seen["College prize"] {
		t.Fatal("independent funding rounds were merged or omitted")
	}

	detail, err := svc.Get(f.ctx, f.identity(), f.team, f.project, f.rounds[0])
	if err != nil {
		t.Fatal(err)
	}
	if detail.FundingSource != "school" || len(detail.Versions) != 1 || detail.Versions[0].Effective || detail.AmountsVisible {
		t.Fatalf("unexpected round detail: %+v", detail)
	}
	if _, err := svc.Get(f.ctx, f.identity(), f.team, f.project, f.rounds[1]); err != nil {
		t.Fatalf("second independent round not readable: %v", err)
	}
}

func TestRoundReadsRequireLiveSessionTeamAndProject(t *testing.T) {
	f := newRoundFixture(t)
	svc := bonusrounds.NewService(bonusrounds.NewRepository(f.db.Pool))
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.db.Pool.Exec(f.ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}

	// A different team cannot read this project's rounds even with a live session.
	otherTeam, otherUser, otherSession := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO users(id,username,password_hash) VALUES($1,'round_outsider','not-a-login-hash')", otherUser)
	exec("INSERT INTO teams(id,name,created_by) VALUES($1,'Other team',$2)", otherTeam, otherUser)
	exec("INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner')", otherTeam, otherUser)
	exec("INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES($1,$2,repeat('f',64),now()+interval '1 hour')", otherSession, otherUser)
	_, err := svc.List(f.ctx, reqctx.Identity{UserID: otherUser, SessionID: otherSession}, f.team, f.project)
	requireNotFound(t, err, "cross-team list")
	_, err = svc.Get(f.ctx, reqctx.Identity{UserID: otherUser, SessionID: otherSession}, f.team, f.project, f.rounds[0])
	requireNotFound(t, err, "cross-team detail")

	// A Team member without project membership has no bonus metadata access.
	teamOnly, teamOnlySession := uuid.New(), uuid.New()
	exec("INSERT INTO users(id,username,password_hash) VALUES($1,'round_team_only','not-a-login-hash')", teamOnly)
	exec("INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'member')", f.team, teamOnly)
	exec("INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES($1,$2,repeat('1',64),now()+interval '1 hour')", teamOnlySession, teamOnly)
	_, err = svc.List(f.ctx, reqctx.Identity{UserID: teamOnly, SessionID: teamOnlySession}, f.team, f.project)
	requireNotFound(t, err, "team member without project membership")

	// ROOT has no implicit bonus visibility.
	root, rootSession := uuid.New(), uuid.New()
	exec("INSERT INTO users(id,username,password_hash,system_root) VALUES($1,'round_root','not-a-login-hash',true)", root)
	exec("INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES($1,$2,repeat('2',64),now()+interval '1 hour')", rootSession, root)
	_, err = svc.List(f.ctx, reqctx.Identity{UserID: root, SessionID: rootSession, SystemRoot: true}, f.team, f.project)
	requireNotFound(t, err, "ROOT without membership")

	// Expired and revoked sessions never authenticate a bonus read.
	expired, revoked := uuid.New(), uuid.New()
	exec("INSERT INTO sessions(id,user_id,token_hash,created_at,expires_at) VALUES($1,$2,repeat('3',64),now()-interval '2 hours',now()-interval '1 hour')", expired, f.member)
	exec("INSERT INTO sessions(id,user_id,token_hash,expires_at,revoked_at) VALUES($1,$2,repeat('4',64),now()+interval '1 hour',now())", revoked, f.member)
	_, err = svc.List(f.ctx, reqctx.Identity{UserID: f.member, SessionID: expired}, f.team, f.project)
	requireNotFound(t, err, "expired session")
	_, err = svc.Get(f.ctx, reqctx.Identity{UserID: f.member, SessionID: revoked}, f.team, f.project, f.rounds[0])
	requireNotFound(t, err, "revoked session")

	// Removing project membership takes effect on the next statement snapshot.
	exec("UPDATE project_members SET status='removed',updated_at=clock_timestamp() WHERE team_id=$1 AND project_id=$2 AND user_id=$3", f.team, f.project, f.member)
	_, err = svc.List(f.ctx, f.identity(), f.team, f.project)
	requireNotFound(t, err, "revoked project membership")
}

func TestRoundFromAnotherProjectOrTeamIsHidden(t *testing.T) {
	f := newRoundFixture(t)
	svc := bonusrounds.NewService(bonusrounds.NewRepository(f.db.Pool))
	otherProject, otherRound := uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.db.Pool.Exec(f.ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Other project',$3,$3)", otherProject, f.team, f.owner)
	exec("INSERT INTO bonus_rounds(id,team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,$3,'school-2026','Other prize','school',$4)", otherRound, f.team, otherProject, f.owner)

	_, err := svc.Get(f.ctx, f.identity(), f.team, f.project, otherRound)
	requireNotFound(t, err, "round from another project")

	// The same Team/Project identifiers exist, but a third Team can never read them.
	thirdTeam := uuid.New()
	exec("INSERT INTO teams(id,name,created_by) VALUES($1,'Third team',$2)", thirdTeam, f.owner)
	_, err = svc.List(f.ctx, f.identity(), thirdTeam, f.project)
	requireNotFound(t, err, "cross-team identifiers")
}
