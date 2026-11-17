//go:build integration

package tasks

import (
	"context"
	"errors"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

type fixture struct {
	svc  *Service
	ps   *projects.Service
	ts   *teams.Service
	as   *auth.Service
	pool *pgxpool.Pool
	ctx  context.Context
	a    reqctx.Identity
	team uuid.UUID
	p    projects.Project
	t    *testing.T
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db := testdb.New(t, os.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if e := database.Up(ctx, db.SQL, logger); e != nil {
		t.Fatal(e)
	}
	ar := audit.NewRepository(db.Pool)
	as := auth.NewService(auth.NewRepository(db.Pool), ar, auth.NewLoginLimiter(1000, time.Minute), time.Hour, logger)
	u, e := as.BootstrapRoot(ctx, "root", "test-strong-password")
	if e != nil {
		t.Fatal(e)
	}
	a := reqctx.Identity{UserID: u.ID, SystemRoot: true}
	tr := teams.NewRepository(db.Pool)
	ts := teams.NewService(tr, as, ar, logger)
	tm, e := ts.CreateTeam(ctx, a, "Team A", "")
	if e != nil {
		t.Fatal(e)
	}
	ps := projects.NewService(projects.NewRepository(db.Pool), tr, ar, logger)
	p, e := ps.Create(ctx, a, tm.ID, projects.CreateInput{ProjectInput: projects.ProjectInput{Name: "Project A"}, LeadID: a.UserID})
	if e != nil {
		t.Fatal(e)
	}
	return &fixture{NewService(NewRepository(), ps), ps, ts, as, db.Pool, ctx, a, tm.ID, p, t}
}
func (f *fixture) create(parent *uuid.UUID, weight int) Task {
	f.t.Helper()
	v, e := f.svc.Create(f.ctx, f.a, f.team, f.p.ID, CreateInput{TaskFields: TaskFields{Title: "Task", OwnerID: f.a.UserID}, ParentID: parent, Weight: weight})
	if e != nil {
		f.t.Fatal(e)
	}
	return v
}
func (f *fixture) user(name string, join bool) reqctx.Identity {
	f.t.Helper()
	u, e := f.as.CreateUser(f.ctx, f.a, name, "", "test-strong-password")
	if e != nil {
		f.t.Fatal(e)
	}
	a := reqctx.Identity{UserID: u.ID}
	if join {
		inv, e := f.ts.Invite(f.ctx, f.a, f.team, name, teams.RoleMember, 1)
		if e != nil {
			f.t.Fatal(e)
		}
		if _, e = f.ts.AcceptInvitation(f.ctx, a, inv.ID); e != nil {
			f.t.Fatal(e)
		}
		if _, e = f.ps.ChangeMember(f.ctx, f.a, f.team, f.p.ID, a.UserID, true); e != nil {
			f.t.Fatal(e)
		}
	}
	return a
}
func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	var api *httpx.APIError
	if !errors.As(err, &api) || api.Status != status {
		t.Fatalf("want status %d, got %v", status, err)
	}
}
func TestTaskHierarchyAndSafeConversion(t *testing.T) {
	f := setup(t)
	one := f.create(nil, 100)
	two := f.create(&one.ID, 10)
	three := f.create(&two.ID, 3)
	_, e := f.svc.Create(f.ctx, f.a, f.team, f.p.ID, CreateInput{TaskFields: three.TaskFields, ParentID: &three.ID, Weight: 1})
	wantStatus(t, e, 409)
	_, e = f.svc.Move(f.ctx, f.a, f.team, f.p.ID, one.ID, &three.ID, one.Version)
	wantStatus(t, e, 409)
	other := f.create(nil, 1)
	other2 := f.create(&other.ID, 1)
	_, e = f.svc.Move(f.ctx, f.a, f.team, f.p.ID, two.ID, &other2.ID, two.Version)
	wantStatus(t, e, 409)
	_, e = f.svc.Progress(f.ctx, f.a, f.team, f.p.ID, one.ID, 1, one.Version)
	wantStatus(t, e, 409)
	_, e = f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, one.ID, InProgress, "", one.Version)
	wantStatus(t, e, 409)
	moved, e := f.svc.Move(f.ctx, f.a, f.team, f.p.ID, two.ID, nil, two.Version)
	if e != nil || moved.Depth != 1 {
		t.Fatalf("move: %+v %v", moved, e)
	}
	got, e := f.svc.Get(f.ctx, f.a, f.team, f.p.ID, three.ID)
	if e != nil || got.Depth != 2 {
		t.Fatal("subtree depth not updated", e)
	}
	run := f.create(nil, 1)
	run, e = f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, run.ID, InProgress, "", run.Version)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.svc.Create(f.ctx, f.a, f.team, f.p.ID, CreateInput{TaskFields: run.TaskFields, ParentID: &run.ID, Weight: 1})
	wantStatus(t, e, 409)
	var history int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_progress_history WHERE team_id=$1 AND task_id=$2 AND event_type='started'`, f.team, run.ID).Scan(&history); e != nil || history != 1 {
		t.Fatal("execution history lost", e)
	}
}
func TestTaskIsolationAssignmentsAndParticipantAccess(t *testing.T) {
	f := setup(t)
	one := f.create(nil, 1)
	other, e := f.ps.Create(f.ctx, f.a, f.team, projects.CreateInput{ProjectInput: projects.ProjectInput{Name: "Other"}, LeadID: f.a.UserID})
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.svc.Create(f.ctx, f.a, f.team, other.ID, CreateInput{TaskFields: one.TaskFields, ParentID: &one.ID, Weight: 1})
	wantStatus(t, e, 404)
	outsider := f.user("outsider", false)
	b, e := f.ts.CreateTeam(f.ctx, outsider, "Team B", "")
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.svc.Get(f.ctx, f.a, b.ID, f.p.ID, one.ID)
	wantStatus(t, e, 404)
	_, e = f.svc.Get(f.ctx, outsider, f.team, f.p.ID, one.ID)
	wantStatus(t, e, 404)
	_, e = f.svc.Create(f.ctx, f.a, f.team, f.p.ID, CreateInput{TaskFields: TaskFields{Title: "invalid owner", OwnerID: outsider.UserID}, Weight: 1})
	wantStatus(t, e, 422)
	member := f.user("member", true)
	_, e = f.svc.Get(f.ctx, member, f.team, f.p.ID, one.ID)
	wantStatus(t, e, 404)
	in := one.TaskFields
	in.CollaboratorIDs = []uuid.UUID{member.UserID}
	one, e = f.svc.Edit(f.ctx, f.a, f.team, f.p.ID, one.ID, EditInput{in, one.Version})
	if e != nil {
		t.Fatal(e)
	}
	tree, e := f.svc.Tree(f.ctx, member, f.team, f.p.ID)
	if e != nil || len(tree.Tasks) != 1 {
		t.Fatal("collaborator cannot read own task", e)
	}
	_, e = f.svc.Edit(f.ctx, member, f.team, f.p.ID, one.ID, EditInput{in, one.Version})
	wantStatus(t, e, 403)
	_, e = f.svc.Transition(f.ctx, member, f.team, f.p.ID, one.ID, Cancelled, "no", one.Version)
	wantStatus(t, e, 403)
	one, e = f.svc.Transition(f.ctx, member, f.team, f.p.ID, one.ID, InProgress, "", one.Version)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.svc.Progress(f.ctx, member, f.team, f.p.ID, one.ID, 50, one.Version)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.ps.ChangeMember(f.ctx, f.a, f.team, f.p.ID, member.UserID, false)
	wantStatus(t, e, 409)
	if e = f.ts.RemoveMember(f.ctx, f.a, f.team, member.UserID); e != nil {
		t.Fatal(e)
	}
	_, e = f.svc.Get(f.ctx, member, f.team, f.p.ID, one.ID)
	wantStatus(t, e, 404)
}
func TestTaskStateMachineAndOptimisticConcurrency(t *testing.T) {
	f := setup(t)
	one := f.create(nil, 1)
	for _, to := range []string{AwaitingReview, Completed, Rejected} {
		_, e := f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, one.ID, to, "", one.Version)
		wantStatus(t, e, 409)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, one.ID, InProgress, "", one.Version)
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	success, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			wantStatus(t, e, 409)
			conflicts++
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("lost update")
	}
	one, e := f.svc.Get(f.ctx, f.a, f.team, f.p.ID, one.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.svc.Progress(f.ctx, f.a, f.team, f.p.ID, one.ID, 100, one.Version)
	wantStatus(t, e, 422)
	one, e = f.svc.Progress(f.ctx, f.a, f.team, f.p.ID, one.ID, 99, one.Version)
	if e != nil || one.EffectiveProgressBPS != 7900 {
		t.Fatal("cap", one, e)
	}
	_, e = f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, one.ID, Cancelled, "", one.Version)
	wantStatus(t, e, 422)
	one, e = f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, one.ID, Cancelled, "scope removed", one.Version)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, one.ID, Completed, "", one.Version)
	wantStatus(t, e, 409)
	p, e := f.svc.ProjectProgress(f.ctx, f.a, f.team, f.p.ID)
	if e != nil || p.ProgressBPS != 0 || p.TotalWeight != "0" {
		t.Fatal("cancel must exclude weight", p, e)
	}
	if _, e = f.pool.Exec(f.ctx, `UPDATE tasks SET status='completed',cancellation_reason='' WHERE team_id=$1 AND id=$2`, f.team, one.ID); e == nil {
		t.Fatal("database permitted unsupported completion")
	}
}
func TestTaskAuditFailureRollsBackStateAndHistory(t *testing.T) {
	f := setup(t)
	one := f.create(nil, 1)
	_, e := f.pool.Exec(f.ctx, `CREATE FUNCTION fail_task_audit() RETURNS trigger AS $$ BEGIN IF NEW.action='task.started' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql; CREATE TRIGGER fail_task_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_task_audit()`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, one.ID, InProgress, "", one.Version)
	wantStatus(t, e, 500)
	got, e := f.svc.Get(f.ctx, f.a, f.team, f.p.ID, one.ID)
	if e != nil || got.Status != Pending || got.Version != 1 {
		t.Fatal("state was not rolled back", e)
	}
	hs, e := f.svc.History(f.ctx, f.a, f.team, f.p.ID, one.ID)
	if e != nil || len(hs) != 1 {
		t.Fatal("history was not rolled back", e)
	}
	ss, e := f.svc.Snapshots(f.ctx, f.a, f.team, f.p.ID)
	if e != nil || len(ss) != 1 {
		t.Fatal("snapshot was not rolled back", e)
	}
}
func TestConcurrentSubtreeMovesPreserveDepth(t *testing.T) {
	f := setup(t)
	a := f.create(nil, 1)
	b := f.create(nil, 1)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, pair := range [][2]Task{{a, b}, {b, a}} {
		wg.Add(1)
		go func(pair [2]Task) {
			defer wg.Done()
			_, e := f.svc.Move(f.ctx, f.a, f.team, f.p.ID, pair[0].ID, &pair[1].ID, pair[0].Version)
			errs <- e
		}(pair)
	}
	wg.Wait()
	close(errs)
	count := 0
	for e := range errs {
		if e == nil {
			count++
		} else {
			wantStatus(t, e, 409)
		}
	}
	if count != 1 {
		t.Fatal("concurrent moves must not create a cycle")
	}
	tree, e := f.svc.Tree(f.ctx, f.a, f.team, f.p.ID)
	if e != nil {
		t.Fatal(e)
	}
	for _, t := range tree.Tasks {
		if t.Depth > 3 {
			f.t.Fatal("depth overflow")
		}
	}
}
