//go:build integration

package submissions

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/tasks"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

type fixture struct {
	sql  *sql.DB
	svc  *tasks.Service
	sub  *Service
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
	svc := tasks.NewService(tasks.NewRepository(), ps)
	return &fixture{sql: db.SQL, svc: svc, sub: NewService(NewRepository(db.Pool), ps, svc), ps: ps, ts: ts, as: as, pool: db.Pool, ctx: ctx, a: a, team: tm.ID, p: p, t: t}
}
func (f *fixture) create(parent *uuid.UUID, weight int) tasks.Task {
	f.t.Helper()
	v, e := f.svc.Create(f.ctx, f.a, f.team, f.p.ID, tasks.CreateInput{TaskFields: tasks.TaskFields{Title: "Task", OwnerID: f.a.UserID}, ParentID: parent, Weight: weight})
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

func (f *fixture) start(v tasks.Task) tasks.Task {
	f.t.Helper()
	v, e := f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, v.ID, tasks.InProgress, "", v.Version)
	if e != nil {
		f.t.Fatal(e)
	}
	return v
}
func (f *fixture) submit(v tasks.Task) Version {
	f.t.Helper()
	out, e := f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, v.ID, SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: v.Version, Summary: "Completed the requested implementation with reproducible evidence"})
	if e != nil {
		f.t.Fatal(e)
	}
	return out
}
func (f *fixture) task(id uuid.UUID) tasks.Task {
	f.t.Helper()
	v, e := f.svc.Get(f.ctx, f.a, f.team, f.p.ID, id)
	if e != nil {
		f.t.Fatal(e)
	}
	return v
}
func (f *fixture) reviewer(name string) reqctx.Identity {
	f.t.Helper()
	u := f.user(name, true)
	if _, e := f.sub.Grant(f.ctx, f.a, f.team, f.p.ID, u.UserID, "Independent verification"); e != nil {
		f.t.Fatal(e)
	}
	return u
}
func TestImmutableVersionReviewProgressDependency(t *testing.T) {
	f := setup(t)
	reviewer := f.reviewer("reviewer")
	parent := f.create(nil, 1)
	first := f.create(&parent.ID, 3)
	next := f.create(nil, 1)
	if _, e := f.svc.AddDependency(f.ctx, f.a, f.team, f.p.ID, next.ID, first.ID, next.Version); e != nil {
		t.Fatal(e)
	}
	first = f.start(first)
	v1 := f.submit(first)
	awaiting := f.task(first.ID)
	if awaiting.Status != tasks.AwaitingReview || awaiting.EffectiveProgressBPS != 8000 || !f.task(next.ID).Blocked {
		t.Fatal(awaiting)
	}
	pending, e := f.sub.Pending(f.ctx, reviewer, f.team)
	if e != nil || len(pending) != 1 {
		t.Fatal(pending, e)
	}
	if _, e = f.sub.Review(f.ctx, reviewer, f.team, f.p.ID, first.ID, v1.ID, ReviewInput{awaiting.Version, "reject", "Add missing acceptance checks"}); e != nil {
		t.Fatal(e)
	}
	rejected := f.task(first.ID)
	if rejected.EffectiveProgressBPS != 0 || rejected.Status != tasks.Rejected {
		t.Fatal(rejected)
	}
	if _, e = f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, first.ID, tasks.InProgress, "", rejected.Version); e == nil {
		t.Fatal("correction requires reason")
	}
	correction, e := f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, first.ID, tasks.InProgress, "Address reviewer feedback", rejected.Version)
	if e != nil {
		t.Fatal(e)
	}
	v2 := f.submit(correction)
	if v1.Version != 1 || v2.Version != 2 || v1.SubmissionID != v2.SubmissionID {
		t.Fatal(v1, v2)
	}
	current := f.task(first.ID)
	_, e = f.sub.Review(f.ctx, reviewer, f.team, f.p.ID, first.ID, v1.ID, ReviewInput{current.Version, "approve", "Stale request"})
	wantStatus(t, e, 409)
	if _, e = f.sub.Review(f.ctx, reviewer, f.team, f.p.ID, first.ID, v2.ID, ReviewInput{current.Version, "approve", "Verified V2"}); e != nil {
		t.Fatal(e)
	}
	done := f.task(first.ID)
	summary := f.task(parent.ID)
	if summary.Status != tasks.Completed || summary.EffectiveProgressBPS != 10000 {
		t.Fatal(summary)
	}
	if done.Status != tasks.Completed || done.EffectiveProgressBPS != 10000 || f.task(next.ID).Blocked {
		t.Fatal(done)
	}
	p, e := f.svc.ProjectProgress(f.ctx, f.a, f.team, f.p.ID)
	if e != nil || p.ProgressBPS != 7500 {
		t.Fatal(p, e)
	}
	versions, e := f.sub.Versions(f.ctx, reviewer, f.team, f.p.ID, first.ID)
	if e != nil || len(versions) != 2 || versions[0].Review.Decision != "approve" || versions[1].Review.Decision != "reject" {
		t.Fatal(versions, e)
	}
	h, e := f.svc.History(f.ctx, reviewer, f.team, f.p.ID, first.ID)
	if e != nil || h[0].EventType != "review_approved" || h[0].AfterProjectBPS != 7500 {
		t.Fatal(h, e)
	}
	for _, table := range []string{"task_submissions", "task_submission_versions", "task_reviews"} {
		if _, e = f.pool.Exec(f.ctx, "DELETE FROM "+table+" WHERE team_id=$1", f.team); e == nil {
			t.Fatalf("%s history deleted", table)
		}
	}
	if _, e = f.svc.Edit(f.ctx, f.a, f.team, f.p.ID, first.ID, tasks.EditInput{TaskFields: done.TaskFields, ExpectedVersion: done.Version}); e == nil {
		t.Fatal("completed task edited")
	}
}
func TestSubmitAndReviewAuthorization(t *testing.T) {
	f := setup(t)
	v := f.start(f.create(nil, 1))
	other := f.user("other", true)
	outside := f.user("outside", false)
	in := SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: v.Version, Summary: "Evidence"}
	_, e := f.sub.Submit(f.ctx, other, f.team, f.p.ID, v.ID, in)
	wantStatus(t, e, 403)
	_, e = f.sub.Submit(f.ctx, outside, f.team, f.p.ID, v.ID, in)
	wantStatus(t, e, 404)
	_, e = f.sub.Submit(f.ctx, f.a, uuid.New(), f.p.ID, v.ID, in)
	wantStatus(t, e, 404)
	_, e = f.sub.Submit(f.ctx, f.a, f.team, uuid.New(), v.ID, in)
	wantStatus(t, e, 404)
	_, e = f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, uuid.New(), in)
	wantStatus(t, e, 404)
	version := f.submit(v)
	now := f.task(v.ID)
	_, e = f.sub.Review(f.ctx, f.a, f.team, f.p.ID, v.ID, version.ID, ReviewInput{now.Version, "approve", "Self"})
	wantStatus(t, e, 403)
	// Team admin has no implicit project review authority, even with ROOT identity.
	if e = f.ts.ChangeMemberRole(f.ctx, f.a, f.team, other.UserID, teams.RoleAdmin); e != nil {
		t.Fatal(e)
	}
	other.SystemRoot = true
	_, e = f.sub.Review(f.ctx, other, f.team, f.p.ID, v.ID, version.ID, ReviewInput{now.Version, "approve", "Admin"})
	if e == nil {
		t.Fatal("team admin bypassed review grant")
	}
	if _, e = f.sub.Grant(f.ctx, f.a, f.team, f.p.ID, other.UserID, "Review evidence"); e != nil {
		t.Fatal(e)
	}
	_, e = f.sub.Review(f.ctx, other, f.team, f.p.ID, v.ID, uuid.New(), ReviewInput{now.Version, "approve", "Unknown"})
	wantStatus(t, e, 404)
	grants, e := f.sub.Reviewers(f.ctx, f.a, f.team, f.p.ID)
	if e != nil {
		t.Fatal(e)
	}
	var grant Reviewer
	for _, r := range grants {
		if r.UserID == other.UserID {
			grant = r
		}
	}
	if _, e = f.sub.Revoke(f.ctx, f.a, f.team, f.p.ID, other.UserID, grant.Version, "Reviewer rotation"); e != nil {
		t.Fatal(e)
	}
	_, e = f.sub.Review(f.ctx, other, f.team, f.p.ID, v.ID, version.ID, ReviewInput{now.Version, "approve", "After revocation"})
	if e == nil {
		t.Fatal("revocation not effective")
	}
}
func TestReviewGrantDoesNotResurrectAfterRejoin(t *testing.T) {
	f := setup(t)
	r := f.reviewer("epoch")
	if _, e := f.ps.ChangeMember(f.ctx, f.a, f.team, f.p.ID, r.UserID, false); e != nil {
		t.Fatal(e)
	}
	if _, e := f.ps.ChangeMember(f.ctx, f.a, f.team, f.p.ID, r.UserID, true); e != nil {
		t.Fatal(e)
	}
	v := f.start(f.create(nil, 1))
	ver := f.submit(v)
	now := f.task(v.ID)
	_, e := f.sub.Review(f.ctx, r, f.team, f.p.ID, v.ID, ver.ID, ReviewInput{now.Version, "approve", "Old grant"})
	if e == nil {
		t.Fatal("old grant resurrected")
	}
	if _, e = f.sub.Grant(f.ctx, f.a, f.team, f.p.ID, r.UserID, "Fresh explicit authority"); e != nil {
		t.Fatal(e)
	}
	if e = f.ts.RemoveMember(f.ctx, f.a, f.team, r.UserID); e != nil {
		t.Fatal(e)
	}
	_, e = f.sub.Review(f.ctx, r, f.team, f.p.ID, v.ID, ver.ID, ReviewInput{now.Version, "approve", "Removed member"})
	wantStatus(t, e, 404)
}
func TestSubmissionStatesAndArchive(t *testing.T) {
	f := setup(t)
	p := f.create(nil, 1)
	child := f.create(&p.ID, 1)
	_, e := f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, p.ID, SubmitInput{uuid.New(), p.Version, "Parent", nil, nil})
	wantStatus(t, e, 409)
	_, e = f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, child.ID, SubmitInput{uuid.New(), child.Version, "Pending", nil, nil})
	wantStatus(t, e, 409)
	child = f.start(child)
	r := f.reviewer("archiver")
	v := f.submit(child)
	now := f.task(child.ID)
	if _, e = f.ps.Archive(f.ctx, f.a, f.team, f.p.ID, f.p.Version); e != nil {
		t.Fatal(e)
	}
	_, e = f.sub.Review(f.ctx, r, f.team, f.p.ID, child.ID, v.ID, ReviewInput{now.Version, "approve", "Archived"})
	wantStatus(t, e, 409)
	_, e = f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, child.ID, SubmitInput{uuid.New(), now.Version, "Archived", nil, nil})
	wantStatus(t, e, 409)
}
func TestSubmissionReplayAndConcurrentReviews(t *testing.T) {
	f := setup(t)
	r := f.reviewer("race")
	v := f.start(f.create(nil, 1))
	in := SubmitInput{uuid.New(), v.Version, "Retryable evidence", nil, nil}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	ids := make(chan uuid.UUID, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ver, e := f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, v.ID, in)
			ids <- ver.ID
			errs <- e
		}()
	}
	wg.Wait()
	for range 2 {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	id1, id2 := <-ids, <-ids
	if id1 != id2 {
		t.Fatal("retry created two versions")
	}
	in.Summary = "Changed content"
	_, e := f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, v.ID, in)
	wantStatus(t, e, 409)
	now := f.task(v.ID)
	success := 0
	for _, decision := range []string{"approve", "reject"} {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			_, e := f.sub.Review(f.ctx, r, f.team, f.p.ID, v.ID, id1, ReviewInput{now.Version, d, "Concurrent decision"})
			errs <- e
		}(decision)
	}
	wg.Wait()
	for range 2 {
		e := <-errs
		if e == nil {
			success++
		} else {
			wantStatus(t, e, 409)
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	reviews, e := f.sub.Reviews(f.ctx, f.a, f.team, f.p.ID, v.ID)
	if e != nil || len(reviews) != 1 {
		t.Fatal(reviews, e)
	}
}
func TestDatabaseRejectsForgedEvidenceAndRollback(t *testing.T) {
	f := setup(t)
	r := f.reviewer("sqlreview")
	v := f.start(f.create(nil, 1))
	for _, state := range []string{"completed", "awaiting_review", "rejected"} {
		if _, e := f.pool.Exec(f.ctx, `UPDATE tasks SET status=$2,version=version+1 WHERE id=$1`, v.ID, state); e == nil {
			t.Fatal("raw state accepted", state)
		}
	}
	ver := f.submit(v)
	now := f.task(v.ID)
	if _, e := f.pool.Exec(f.ctx, `UPDATE tasks SET status='completed',version=version+1 WHERE id=$1`, v.ID); e == nil {
		t.Fatal("no approval accepted")
	}
	// Fail the final audit insert: all prior writes in the review transaction must roll back.
	_, e := f.pool.Exec(f.ctx, `CREATE FUNCTION fail_review_audit() RETURNS trigger AS $$ BEGIN IF NEW.action='submission.reviewed' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql; CREATE TRIGGER fail_review_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_review_audit()`)
	if e != nil {
		t.Fatal(e)
	}
	var before int
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM project_progress_snapshots WHERE team_id=$1`, f.team).Scan(&before)
	_, e = f.sub.Review(f.ctx, r, f.team, f.p.ID, v.ID, ver.ID, ReviewInput{now.Version, "approve", "Must roll back"})
	if e == nil {
		t.Fatal("injected failure did not fail")
	}
	after := f.task(v.ID)
	reviews, e := f.sub.Reviews(f.ctx, f.a, f.team, f.p.ID, v.ID)
	var snapshots, audits int
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM project_progress_snapshots WHERE team_id=$1`, f.team).Scan(&snapshots)
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_events WHERE team_id=$1 AND action IN ('task.review_approved','submission.reviewed')`, f.team).Scan(&audits)
	if e != nil || len(reviews) != 0 || after.Version != now.Version || after.Status != tasks.AwaitingReview || snapshots != before || audits != 0 {
		t.Fatal(after, reviews, e, snapshots, before, audits)
	}
	if _, e = f.pool.Exec(f.ctx, `DROP TRIGGER fail_review_audit ON audit_events`); e != nil {
		t.Fatal(e)
	}
	// Force commit of an evidence-free version in a transaction: deferred guards refuse it.
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(f.ctx)
	_, e = tx.Exec(f.ctx, `UPDATE tasks SET status='completed',accepted_review_id=$2,version=version+1 WHERE id=$1`, v.ID, uuid.New())
	if e == nil {
		e = tx.Commit(f.ctx)
	}
	if e == nil {
		t.Fatal("forged review ID committed")
	}
}
func TestDefaultLeadCanReviewOtherAuthor(t *testing.T) {
	f := setup(t)
	owner := f.user("author", true)
	v := f.create(nil, 1)
	fields := v.TaskFields
	fields.OwnerID = owner.UserID
	v, e := f.svc.Edit(f.ctx, f.a, f.team, f.p.ID, v.ID, tasks.EditInput{TaskFields: fields, ExpectedVersion: v.Version})
	if e != nil {
		t.Fatal(e)
	}
	v, e = f.svc.Transition(f.ctx, owner, f.team, f.p.ID, v.ID, tasks.InProgress, "", v.Version)
	if e != nil {
		t.Fatal(e)
	}
	ver, e := f.sub.Submit(f.ctx, owner, f.team, f.p.ID, v.ID, SubmitInput{uuid.New(), v.Version, "Owner evidence", nil, nil})
	if e != nil {
		t.Fatal(e)
	}
	v = f.task(v.ID)
	if _, e = f.sub.Review(f.ctx, f.a, f.team, f.p.ID, v.ID, ver.ID, ReviewInput{v.Version, "approve", "Lead verification"}); e != nil {
		t.Fatal(e)
	}
}

func TestPhase04RollbackRefusesBusinessEvidence(t *testing.T) {
	f := setup(t)
	f.submit(f.start(f.create(nil, 1)))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if e := database.Down(f.ctx, f.sql, 1, logger); e == nil {
		t.Fatal("rollback deleted immutable evidence")
	}
	v, e := database.Version(f.ctx, f.sql)
	if e != nil || v != 6 {
		t.Fatal(v, e)
	}
}
func TestRejectedDirectResubmissionAndCancelledDependencies(t *testing.T) {
	f := setup(t)
	r := f.reviewer("retry")
	first, next := f.create(nil, 1), f.create(nil, 1)
	if _, e := f.svc.AddDependency(f.ctx, f.a, f.team, f.p.ID, next.ID, first.ID, next.Version); e != nil {
		t.Fatal(e)
	}
	first = f.start(first)
	v1 := f.submit(first)
	now := f.task(first.ID)
	if _, e := f.sub.Review(f.ctx, r, f.team, f.p.ID, first.ID, v1.ID, ReviewInput{now.Version, "reject", "Correct evidence"}); e != nil {
		t.Fatal(e)
	}
	v2 := f.submit(f.task(first.ID))
	if v2.Version != 2 {
		t.Fatal(v2)
	}
	now = f.task(first.ID)
	if _, e := f.sub.Review(f.ctx, r, f.team, f.p.ID, first.ID, v2.ID, ReviewInput{now.Version, "reject", "Still incomplete"}); e != nil {
		t.Fatal(e)
	}
	now = f.task(first.ID)
	if _, e := f.svc.Transition(f.ctx, f.a, f.team, f.p.ID, first.ID, tasks.Cancelled, "Cancelled after rejection", now.Version); e != nil {
		t.Fatal(e)
	}
	if !f.task(next.ID).Blocked {
		t.Fatal("cancelled predecessor unblocked task")
	}
	_, e := f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, first.ID, SubmitInput{uuid.New(), now.Version + 1, "Cancelled", nil, nil})
	wantStatus(t, e, 409)
}
