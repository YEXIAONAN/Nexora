//go:build integration

package projects

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

type fixture struct {
	ctx        context.Context
	s          *Service
	t          *teams.Service
	a          *auth.Service
	pool       *pgxpool.Pool
	root, user reqctx.Identity
	team       uuid.UUID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db := testdb.New(t, os.Getenv)
	ctx := context.Background()
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	if e := database.Up(ctx, db.SQL, l); e != nil {
		t.Fatal(e)
	}
	ar := audit.NewRepository(db.Pool)
	a := auth.NewService(auth.NewRepository(db.Pool), ar, auth.NewLoginLimiter(100, time.Minute), time.Hour, l)
	root, e := a.BootstrapRoot(ctx, "root", "project-test-password")
	if e != nil {
		t.Fatal(e)
	}
	r := reqctx.Identity{UserID: root.ID, SystemRoot: true}
	u, e := a.CreateUser(ctx, r, "alice", "", "project-test-password")
	if e != nil {
		t.Fatal(e)
	}
	user := reqctx.Identity{UserID: u.ID}
	tr := teams.NewRepository(db.Pool)
	ts := teams.NewService(tr, a, ar, l)
	team, e := ts.CreateTeam(ctx, r, "A", "")
	if e != nil {
		t.Fatal(e)
	}
	inv, e := ts.Invite(ctx, r, team.ID, "alice", "member", 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ts.AcceptInvitation(ctx, user, inv.ID); e != nil {
		t.Fatal(e)
	}
	return &fixture{ctx, NewService(NewRepository(db.Pool), tr, ar, l), ts, a, db.Pool, r, user, team.ID}
}
func status(t *testing.T, e error, want int) {
	t.Helper()
	var a *httpx.APIError
	if !errors.As(e, &a) || a.Status != want {
		t.Fatalf("want %d got %v", want, e)
	}
}
func create(t *testing.T, f *fixture) Project {
	t.Helper()
	p, e := f.s.Create(f.ctx, f.root, f.team, CreateInput{ProjectInput: ProjectInput{Name: "Project"}, LeadID: f.root.UserID})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestProjectIsolationPermissionsAndQualification(t *testing.T) {
	f := setup(t)
	p := create(t, f)
	other, e := f.t.CreateTeam(f.ctx, f.user, "B", "")
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.Get(f.ctx, f.root, other.ID, p.ID)
	status(t, e, 404)
	_, e = f.s.Get(f.ctx, f.user, other.ID, p.ID)
	status(t, e, 404)
	_, e = f.s.Edit(f.ctx, f.user, f.team, p.ID, EditInput{ProjectInput: ProjectInput{Name: "forged"}, ExpectedVersion: 1})
	status(t, e, 403)
	_, e = f.s.Create(f.ctx, f.user, f.team, CreateInput{ProjectInput: ProjectInput{Name: "bad"}, LeadID: f.user.UserID})
	status(t, e, 403)
	_, e = f.s.Create(f.ctx, f.root, f.team, CreateInput{ProjectInput: ProjectInput{Name: "bad"}, LeadID: uuid.New()})
	status(t, e, 422)
	if _, e = f.s.ChangeMember(f.ctx, f.root, f.team, p.ID, f.user.UserID, true); e != nil {
		t.Fatal(e)
	}
	if e = f.t.RemoveMember(f.ctx, f.root, f.team, f.user.UserID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.Get(f.ctx, f.user, f.team, p.ID)
	status(t, e, 404)
	members, e := f.s.Members(f.ctx, f.root, f.team, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range members {
		if m.UserID == f.user.UserID && m.Active {
			t.Fatal("removed team member still active in project")
		}
	}
}
func TestHandoffArchiveMilestonesAndVersions(t *testing.T) {
	f := setup(t)
	p := create(t, f)
	e := f.t.RemoveMember(f.ctx, f.root, f.team, f.root.UserID)
	status(t, e, 409)
	m, e := f.s.SaveMilestone(f.ctx, f.root, f.team, p.ID, uuid.Nil, MilestoneInput{Name: "Launch"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.SaveMilestone(f.ctx, f.root, f.team, p.ID, m.ID, MilestoneInput{Name: "Launch 2", ExpectedVersion: 99})
	status(t, e, 409)
	_, e = f.s.Handoff(f.ctx, f.root, f.team, p.ID, f.user.UserID, p.Version, "")
	status(t, e, 422)
	p, e = f.s.Handoff(f.ctx, f.root, f.team, p.ID, f.user.UserID, p.Version, "controlled handoff")
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.Edit(f.ctx, f.root, f.team, p.ID, EditInput{ProjectInput: ProjectInput{Name: "no"}, ExpectedVersion: p.Version})
	status(t, e, 403)
	p, e = f.s.Edit(f.ctx, f.user, f.team, p.ID, EditInput{ProjectInput: ProjectInput{Name: "Updated"}, ExpectedVersion: p.Version})
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.Archive(f.ctx, f.user, f.team, p.ID, p.Version-1)
	status(t, e, 409)
	p, e = f.s.Archive(f.ctx, f.user, f.team, p.ID, p.Version)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.SaveMilestone(f.ctx, f.user, f.team, p.ID, uuid.Nil, MilestoneInput{Name: "late"})
	status(t, e, 409)
	_, e = f.s.ChangeMember(f.ctx, f.user, f.team, p.ID, f.root.UserID, false)
	status(t, e, 409)
	_, e = f.s.Handoff(f.ctx, f.root, f.team, p.ID, f.root.UserID, p.Version, "late")
	status(t, e, 409)
	e = f.t.RemoveMember(f.ctx, f.user, f.team, f.user.UserID)
	status(t, e, 409)
}
func TestProjectAuditFailureRollsBack(t *testing.T) {
	f := setup(t)
	p := create(t, f)
	_, e := f.pool.Exec(f.ctx, `CREATE FUNCTION fail_project_audit() RETURNS trigger AS $$ BEGIN IF NEW.action='project.edited' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql; CREATE TRIGGER fail_project_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_project_audit()`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.Edit(f.ctx, f.root, f.team, p.ID, EditInput{ProjectInput: ProjectInput{Name: "rolled back"}, ExpectedVersion: p.Version})
	status(t, e, 500)
	got, e := f.s.Get(f.ctx, f.root, f.team, p.ID)
	if e != nil || got.Name != p.Name || got.Version != p.Version {
		t.Fatalf("non-atomic edit: %+v %v", got, e)
	}
}
func TestRemovedMemberCannotLeadAndForeignAdminCannotEdit(t *testing.T) {
	f := setup(t)
	create(t, f)
	if e := f.t.RemoveMember(f.ctx, f.root, f.team, f.user.UserID); e != nil {
		t.Fatal(e)
	}
	_, e := f.s.Create(f.ctx, f.root, f.team, CreateInput{ProjectInput: ProjectInput{Name: "invalid lead"}, LeadID: f.user.UserID})
	status(t, e, 422)
	other, e := f.t.CreateTeam(f.ctx, f.user, "Other", "")
	if e != nil {
		t.Fatal(e)
	}
	p, e := f.s.Create(f.ctx, f.user, other.ID, CreateInput{ProjectInput: ProjectInput{Name: "Other project"}, LeadID: f.user.UserID})
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.Edit(f.ctx, f.root, other.ID, p.ID, EditInput{ProjectInput: ProjectInput{Name: "tampered"}, ExpectedVersion: p.Version})
	status(t, e, 404)
}
