//go:build integration

package database_test

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/submissions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/tasks"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var p05MigrationFS embed.FS

const p05Password = "integration-fixture-password"

type contributionFixture struct {
	t                                          *testing.T
	ctx                                        context.Context
	db                                         *testdb.Fixture
	log                                        *slog.Logger
	as                                         *auth.Service
	ts                                         *teams.Service
	ps                                         *projects.Service
	tasks                                      *tasks.Service
	sub                                        *submissions.Service
	root, worker, appraiser, arbiter, observer reqctx.Identity
	team                                       uuid.UUID
	project                                    projects.Project
	task                                       tasks.Task
	submission                                 submissions.Version
	review                                     submissions.Review
}

func p05Must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func p05Denied(t *testing.T, e error) {
	t.Helper()
	if e == nil {
		t.Fatal("invalid database mutation succeeded")
	}
	var pg *pgconn.PgError
	if errors.As(e, &pg) && pg.Code == "40P01" {
		t.Fatal("database deadlock", pg.Code)
	}
}
func newContributionFixture(t *testing.T, old bool) *contributionFixture {
	t.Helper()
	f := &contributionFixture{t: t, db: testdb.New(t, os.Getenv), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	if old {
		goose.SetBaseFS(p05MigrationFS)
		p05Must(t, goose.SetDialect("postgres"))
		p05Must(t, goose.UpToContext(f.ctx, f.db.SQL, "migrations", 6))
	} else {
		p05Must(t, database.Up(f.ctx, f.db.SQL, f.log))
	}
	ar := audit.NewRepository(f.db.Pool)
	f.as = auth.NewService(auth.NewRepository(f.db.Pool), ar, auth.NewLoginLimiter(1000, time.Minute), time.Hour, f.log)
	root, e := f.as.BootstrapRoot(f.ctx, "root", p05Password)
	p05Must(t, e)
	f.root = reqctx.Identity{UserID: root.ID, SystemRoot: true}
	tr := teams.NewRepository(f.db.Pool)
	f.ts = teams.NewService(tr, f.as, ar, f.log)
	tm, e := f.ts.CreateTeam(f.ctx, f.root, "Contribution tests", "")
	p05Must(t, e)
	f.team = tm.ID
	f.ps = projects.NewService(projects.NewRepository(f.db.Pool), tr, ar, f.log)
	f.project, e = f.ps.Create(f.ctx, f.root, f.team, projects.CreateInput{ProjectInput: projects.ProjectInput{Name: "Contribution project"}, LeadID: f.root.UserID})
	p05Must(t, e)
	f.worker = f.user("worker", true)
	f.appraiser = f.user("appraiser", true)
	f.arbiter = f.user("arbiter", true)
	f.observer = f.user("observer", true)
	f.tasks = tasks.NewService(tasks.NewRepository(), f.ps)
	f.sub = submissions.NewService(submissions.NewRepository(f.db.Pool), f.ps, f.tasks)
	store, e := submissions.OpenStore(t.TempDir())
	p05Must(t, e)
	t.Cleanup(func() { store.Close() })
	f.sub.SetStore(store)
	f.task, e = f.tasks.Create(f.ctx, f.root, f.team, f.project.ID, tasks.CreateInput{TaskFields: tasks.TaskFields{Title: "Accepted delivery", OwnerID: f.worker.UserID}, Weight: 3})
	p05Must(t, e)
	f.task, e = f.tasks.Transition(f.ctx, f.worker, f.team, f.project.ID, f.task.ID, tasks.InProgress, "", f.task.Version)
	p05Must(t, e)
	file, e := f.sub.Upload(f.ctx, f.worker, f.team, f.project.ID, f.task.ID, uuid.New(), "evidence.txt", strings.NewReader("Real Phase04 immutable file evidence"))
	p05Must(t, e)
	f.submission, e = f.sub.Submit(f.ctx, f.worker, f.team, f.project.ID, f.task.ID, submissions.SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: f.task.Version, Summary: "Completed delivery", AttachmentIDs: []uuid.UUID{file.ID}})
	p05Must(t, e)
	f.task, e = f.tasks.Get(f.ctx, f.root, f.team, f.project.ID, f.task.ID)
	p05Must(t, e)
	f.review, e = f.sub.Review(f.ctx, f.root, f.team, f.project.ID, f.task.ID, f.submission.ID, submissions.ReviewInput{ExpectedVersion: f.task.Version, Decision: "approve", Comment: "Independent task acceptance"})
	p05Must(t, e)
	return f
}
func (f *contributionFixture) user(name string, join bool) reqctx.Identity {
	f.t.Helper()
	u, e := f.as.CreateUser(f.ctx, f.root, name, name, p05Password)
	p05Must(f.t, e)
	a := reqctx.Identity{UserID: u.ID}
	if join {
		inv, e := f.ts.Invite(f.ctx, f.root, f.team, name, teams.RoleMember, 1)
		p05Must(f.t, e)
		_, e = f.ts.AcceptInvitation(f.ctx, a, inv.ID)
		p05Must(f.t, e)
		_, e = f.ps.ChangeMember(f.ctx, f.root, f.team, f.project.ID, u.ID, true)
		p05Must(f.t, e)
	}
	return a
}
func (f *contributionFixture) rev(c uuid.UUID) int {
	f.t.Helper()
	var n int
	p05Must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT revision FROM contribution_cases WHERE id=$1`, c).Scan(&n))
	return n
}
func (f *contributionFixture) latest(c uuid.UUID) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	p05Must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT latest_version_id FROM contribution_cases WHERE id=$1`, c).Scan(&id))
	return id
}
func (f *contributionFixture) create() uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	p05Must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT contribution_create_case($1,$2,$3,$4,$5,$6)`, f.team, f.project.ID, f.task.ID, f.review.ID, f.root.UserID, uuid.NewString()).Scan(&id))
	return id
}
func (f *contributionFixture) grant(a reqctx.Identity) {
	f.t.Helper()
	_, e := f.db.Pool.Exec(f.ctx, `SELECT contribution_set_appraiser($1,$2,$3,$4,'Independent appraisal',$5,0,false)`, f.team, f.project.ID, a.UserID, f.root.UserID, uuid.NewString())
	p05Must(f.t, e)
}
func (f *contributionFixture) share(c uuid.UUID, bps int) {
	f.t.Helper()
	_, e := f.db.Pool.Exec(f.ctx, `SELECT contribution_set_share($1,$2,$3,$4,$5,$6,'Observed delivery','Personal statement',$7,$8,$9,'Manual allocation',$10)`, f.team, f.project.ID, c, f.latest(c), f.worker.UserID, bps, `[{"kind":"objective_fact","source":"accepted_submission","reference":"`+f.submission.ID.String()+`"}]`, f.root.UserID, f.rev(c), uuid.NewString())
	p05Must(f.t, e)
}
func (f *contributionFixture) transition(c uuid.UUID, to string) error {
	_, e := f.db.Pool.Exec(f.ctx, `SELECT contribution_transition($1,$2,$3,$4,$5,$6,'Explicit lifecycle action',$7)`, f.team, f.project.ID, c, f.root.UserID, f.rev(c), to, uuid.NewString())
	return e
}
func (f *contributionFixture) prepare(c uuid.UUID) {
	f.t.Helper()
	f.share(c, 10000)
	p05Must(f.t, f.transition(c, "pending_review"))
}
func (f *contributionFixture) confirm(c uuid.UUID, a reqctx.Identity) (uuid.UUID, error) {
	var id uuid.UUID
	e := f.db.Pool.QueryRow(f.ctx, `SELECT contribution_confirm($1,$2,$3,$4,$5,'Verified allocation',$6)`, f.team, f.project.ID, c, a.UserID, f.rev(c), uuid.NewString()).Scan(&id)
	return id, e
}
func (f *contributionFixture) revise(c uuid.UUID) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	p05Must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT contribution_revise($1,$2,$3,$4,$5,'New evidence requires revision',$6)`, f.team, f.project.ID, c, f.root.UserID, f.rev(c), uuid.NewString()).Scan(&id))
	return id
}
func (f *contributionFixture) dispute(c, confirmation uuid.UUID) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	p05Must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT contribution_raise_dispute($1,$2,$3,$4,$5,$6,'Please independently reconsider',$7,NULL,NULL)`, f.team, f.project.ID, c, confirmation, f.worker.UserID, f.rev(c), uuid.NewString()).Scan(&id))
	return id
}
func (f *contributionFixture) resolve(c, d uuid.UUID, a reqctx.Identity, decision string) error {
	_, e := f.db.Pool.Exec(f.ctx, `SELECT contribution_resolve($1,$2,$3,$4,$5,$6,$7,'Independent ruling',$8)`, f.team, f.project.ID, c, d, a.UserID, f.rev(c), decision, uuid.NewString())
	return e
}
func (f *contributionFixture) official(c uuid.UUID) int {
	f.t.Helper()
	var n int
	p05Must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_effective_shares WHERE case_id=$1`, c).Scan(&n))
	return n
}
func (f *contributionFixture) state(c uuid.UUID) string {
	f.t.Helper()
	var state string
	p05Must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT case_workflow_status FROM contribution_cases WHERE id=$1`, c).Scan(&state))
	return state
}
func (f *contributionFixture) archive() error {
	_, e := f.ps.Archive(f.ctx, f.root, f.team, f.project.ID, f.project.Version)
	return e
}
func (f *contributionFixture) notice(c, conf uuid.UUID, channel string) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	p05Must(f.t, f.db.Pool.QueryRow(f.ctx, `INSERT INTO contribution_notifications(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,recipient_id,channel,created_by,evidence_type,evidence_ref,evidence_digest,claimed_sent_at,request_id) SELECT $1,$2,$3,$4,case_version_id,id,$5,$6,$7,'claimed_receipt','untrusted-manual-reference',version_digest,clock_timestamp(),$8 FROM contribution_confirmations WHERE id=$9 RETURNING id`, f.team, f.project.ID, f.task.ID, c, f.worker.UserID, channel, f.root.UserID, uuid.NewString(), conf).Scan(&id))
	return id
}
func (f *contributionFixture) receipt(notice uuid.UUID, token string) error {
	_, e := f.db.Pool.Exec(f.ctx, `INSERT INTO contribution_attestations(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,purpose,notice_id,subject_id,actor_id,content_digest,proof_token,request_id) SELECT n.team_id,n.project_id,n.task_id,n.case_id,n.case_version_id,n.confirmation_id,'delivery',n.id,n.recipient_id,n.recipient_id,f.version_digest,$2,$3 FROM contribution_notifications n JOIN contribution_confirmations f ON f.id=n.confirmation_id WHERE n.id=$1`, notice, token, uuid.NewString())
	return e
}
func TestContributionDatabaseLifecycle(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	f.grant(f.arbiter)
	c := f.create()
	f.share(c, 4000)
	p05Denied(t, f.transition(c, "pending_review"))
	_, err := f.confirm(c, f.appraiser)
	p05Denied(t, err)
	if f.state(c) != "draft" || f.official(c) != 0 {
		t.Fatal("partial draft became official")
	}
	f.prepare(c)
	conf, e := f.confirm(c, f.appraiser)
	p05Must(t, e)
	v1 := f.latest(c)
	if f.state(c) != "confirmed" || f.official(c) != 1 {
		t.Fatal("confirmation not effective")
	}
	var cases, confirmed int
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT case_count,confirmed_case_count FROM contribution_project_counts WHERE team_id=$1 AND project_id=$2`, f.team, f.project.ID).Scan(&cases, &confirmed))
	if cases != 1 || confirmed != 1 {
		t.Fatal("project case counts disagree with effective confirmations")
	}
	for _, q := range []string{`UPDATE contribution_shares SET share_bps=1 WHERE case_id=$1`, `UPDATE contribution_confirmations SET rationale='changed' WHERE case_id=$1`, `DELETE FROM contribution_confirmations WHERE case_id=$1`, `UPDATE contribution_case_versions SET status='overturned' WHERE case_id=$1`, `UPDATE contribution_cases SET case_workflow_status='draft',revision=revision+1 WHERE id=$1`} {
		_, e = f.db.Pool.Exec(f.ctx, q, c)
		p05Denied(t, e)
	}
	v2 := f.revise(c)
	if f.official(c) != 1 || f.state(c) != "revising" {
		t.Fatal("working draft replaced effective result")
	}
	f.prepare(c)
	_, e = f.confirm(c, f.appraiser)
	p05Must(t, e)
	var oldStatus string
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT status FROM contribution_case_versions WHERE id=$1`, v1).Scan(&oldStatus))
	if oldStatus != "superseded" || f.latest(c) != v2 {
		t.Fatal("historical confirmation lost")
	}
	currentConf := uuid.Nil
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT id FROM contribution_confirmations WHERE case_version_id=$1`, v2).Scan(&currentConf))
	d := f.dispute(c, currentConf)
	if f.official(c) != 0 || f.state(c) != "disputed" {
		t.Fatal("disputed allocation counted")
	}
	p05Denied(t, f.resolve(c, d, f.appraiser, "overturn"))
	p05Denied(t, f.resolve(c, d, f.worker, "uphold"))
	p05Must(t, f.resolve(c, d, f.arbiter, "uphold"))
	if f.official(c) != 1 || f.state(c) != "confirmed" {
		t.Fatal("uphold changed the allocation")
	}
	d = f.dispute(c, currentConf)
	f.reviseBlocked(t, c)
	p05Must(t, f.resolve(c, d, f.arbiter, "overturn"))
	if f.official(c) != 0 || f.state(c) != "draft" {
		t.Fatal("overturn did not clear effectiveness and create a draft")
	}
	var retained int
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_confirmations WHERE case_id=$1 AND id IN ($2,$3)`, c, conf, currentConf).Scan(&retained))
	if retained != 2 {
		t.Fatal("confirmation history erased")
	}
	if _, e = f.db.Pool.Exec(f.ctx, `DELETE FROM contribution_dispute_resolutions WHERE case_id=$1`, c); e == nil {
		t.Fatal("ruling deleted")
	}
}
func (f *contributionFixture) reviseBlocked(t *testing.T, c uuid.UUID) {
	t.Helper()
	_, e := f.db.Pool.Exec(f.ctx, `SELECT contribution_revise($1,$2,$3,$4,$5,'Cannot revise while disputed',$6)`, f.team, f.project.ID, c, f.root.UserID, f.rev(c), uuid.NewString())
	p05Denied(t, e)
}
func TestContributionArchiveAndVerifiedDelivery(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	f.grant(f.arbiter)
	err := f.archive()
	var apiErr *httpx.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
		t.Fatalf("unsettled archive must return the existing conflict response: %v", err)
	}
	c := f.create()
	p05Denied(t, f.archive())
	p05Must(t, f.transition(c, "abandoned"))
	p05Denied(t, f.archive())
	f.revise(c)
	f.prepare(c)
	p05Denied(t, f.archive())
	conf, e := f.confirm(c, f.appraiser)
	p05Must(t, e)
	p05Denied(t, f.archive())
	notice := f.notice(c, conf, "manual_unverified")
	login, e := f.as.Login(f.ctx, "worker", p05Password, "M2 receipt test", "127.0.0.1")
	p05Must(t, e)
	p05Denied(t, f.receipt(notice, login.Token))
	p05Denied(t, f.archive())
	notice = f.notice(c, conf, "authenticated_ack")
	p05Denied(t, f.receipt(notice, "forged-proof"))
	p05Denied(t, f.archive())
	p05Must(t, f.receipt(notice, login.Token))
	var clean bool
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT proof_token IS NULL AND sent_at IS NOT NULL AND acknowledged_at IS NOT NULL AND session_id=$2 FROM contribution_attestations WHERE notice_id=$1`, notice, login.SessionID).Scan(&clean))
	if !clean {
		t.Fatal("receipt lacks authenticated identity or retained a raw token")
	}
	revision := f.revise(c)
	p05Denied(t, f.archive())
	p05Must(t, f.transition(c, "abandoned"))
	if f.latest(c) != revision || f.official(c) != 1 {
		t.Fatal("abandoned revision lost prior effective result")
	}
	d := f.dispute(c, conf)
	p05Denied(t, f.archive())
	p05Must(t, f.resolve(c, d, f.arbiter, "uphold"))
	p05Must(t, f.archive())
	_, e = f.db.Pool.Exec(f.ctx, `SELECT contribution_revise($1,$2,$3,$4,$5,'archived',$6)`, f.team, f.project.ID, c, f.root.UserID, f.rev(c), uuid.NewString())
	p05Denied(t, e)
}

func TestContributionSourceScopeAndMembership(t *testing.T) {
	f := newContributionFixture(t, false)
	c := f.create()
	if f.create() != c {
		t.Fatal("review idempotency changed case")
	}
	for _, args := range [][]any{{f.team, f.project.ID, f.task.ID, uuid.New(), f.root.UserID, uuid.NewString()}, {f.team, uuid.New(), f.task.ID, f.review.ID, f.root.UserID, uuid.NewString()}, {uuid.New(), f.project.ID, f.task.ID, f.review.ID, f.root.UserID, uuid.NewString()}, {f.team, f.project.ID, uuid.New(), f.review.ID, f.root.UserID, uuid.NewString()}} {
		var id uuid.UUID
		e := f.db.Pool.QueryRow(f.ctx, `SELECT contribution_create_case($1,$2,$3,$4,$5,$6)`, args...).Scan(&id)
		p05Denied(t, e)
	}
	_, e := f.db.Pool.Exec(f.ctx, `INSERT INTO contribution_cases(team_id,project_id,task_id,submission_version_id,review_id,created_by,last_actor_id,last_reason,request_id) VALUES($1,$2,$3,$4,$5,$6,$6,'duplicate review',$7)`, f.team, f.project.ID, f.task.ID, f.submission.ID, f.review.ID, f.root.UserID, uuid.NewString())
	p05Denied(t, e)
	outsider := f.user("outsider", false)
	_, e = f.db.Pool.Exec(f.ctx, `SELECT contribution_set_share($1,$2,$3,$4,$5,10000,'basis','statement','[]',$6,$7,'no membership',$8)`, f.team, f.project.ID, c, f.latest(c), outsider.UserID, f.root.UserID, f.rev(c), uuid.NewString())
	p05Denied(t, e)
	var n int
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM project_members WHERE team_id=$1 AND project_id=$2 AND user_id=$3`, f.team, f.project.ID, outsider.UserID).Scan(&n))
	if n != 0 {
		t.Fatal("fake project membership was fabricated")
	}
	other, e := f.ps.Create(f.ctx, f.root, f.team, projects.CreateInput{ProjectInput: projects.ProjectInput{Name: "Other scope"}, LeadID: f.root.UserID})
	p05Must(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `INSERT INTO contribution_case_versions(team_id,project_id,task_id,case_id,version,proposed_by,changed_by,change_reason,request_id) VALUES($1,$2,$3,$4,2,$5,$5,'cross project',$6)`, f.team, other.ID, f.task.ID, c, f.root.UserID, uuid.NewString())
	p05Denied(t, e)
	task, e := f.tasks.Create(f.ctx, f.root, f.team, f.project.ID, tasks.CreateInput{TaskFields: tasks.TaskFields{Title: "Rejected source", OwnerID: f.worker.UserID}, Weight: 1})
	p05Must(t, e)
	task, e = f.tasks.Transition(f.ctx, f.worker, f.team, f.project.ID, task.ID, tasks.InProgress, "", task.Version)
	p05Must(t, e)
	v1, e := f.sub.Submit(f.ctx, f.worker, f.team, f.project.ID, task.ID, submissions.SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: task.Version, Summary: "V1"})
	p05Must(t, e)
	task, e = f.tasks.Get(f.ctx, f.root, f.team, f.project.ID, task.ID)
	p05Must(t, e)
	rejected, e := f.sub.Review(f.ctx, f.root, f.team, f.project.ID, task.ID, v1.ID, submissions.ReviewInput{ExpectedVersion: task.Version, Decision: "reject", Comment: "Fix V1"})
	p05Must(t, e)
	var id uuid.UUID
	e = f.db.Pool.QueryRow(f.ctx, `SELECT contribution_create_case($1,$2,$3,$4,$5,$6)`, f.team, f.project.ID, task.ID, rejected.ID, f.root.UserID, uuid.NewString()).Scan(&id)
	p05Denied(t, e)
	task, e = f.tasks.Get(f.ctx, f.worker, f.team, f.project.ID, task.ID)
	p05Must(t, e)
	v2, e := f.sub.Submit(f.ctx, f.worker, f.team, f.project.ID, task.ID, submissions.SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: task.Version, Summary: "Corrected V2"})
	p05Must(t, e)
	task, e = f.tasks.Get(f.ctx, f.root, f.team, f.project.ID, task.ID)
	p05Must(t, e)
	_, e = f.sub.Review(f.ctx, f.root, f.team, f.project.ID, task.ID, v2.ID, submissions.ReviewInput{ExpectedVersion: task.Version, Decision: "approve", Comment: "V2 verified"})
	p05Must(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `INSERT INTO contribution_cases(team_id,project_id,task_id,submission_version_id,review_id,created_by,last_actor_id,last_reason,request_id) VALUES($1,$2,$3,$4,$5,$6,$6,'stale review',$7)`, f.team, f.project.ID, task.ID, v2.ID, rejected.ID, f.root.UserID, uuid.NewString())
	p05Denied(t, e)
}
func TestContributionAppraiserIndependenceAndEpoch(t *testing.T) {
	f := newContributionFixture(t, false)
	c := f.create()
	f.prepare(c)
	for _, a := range []reqctx.Identity{f.root, f.appraiser, f.observer, {UserID: uuid.New()}} {
		_, e := f.confirm(c, a)
		p05Denied(t, e)
	}
	_, e := f.db.Pool.Exec(f.ctx, `SELECT contribution_set_appraiser($1,$2,$3,$3,'self grant',$4,0,false)`, f.team, f.project.ID, f.root.UserID, uuid.NewString())
	p05Denied(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `SELECT contribution_set_appraiser($1,$2,$3,$4,'missing expected version',$5,NULL,false)`, f.team, f.project.ID, f.appraiser.UserID, f.root.UserID, uuid.NewString())
	p05Denied(t, e)
	f.grant(f.appraiser)
	_, e = f.ps.ChangeMember(f.ctx, f.root, f.team, f.project.ID, f.appraiser.UserID, false)
	p05Must(t, e)
	_, e = f.ps.ChangeMember(f.ctx, f.root, f.team, f.project.ID, f.appraiser.UserID, true)
	p05Must(t, e)
	_, e = f.confirm(c, f.appraiser)
	p05Denied(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `SELECT contribution_set_appraiser($1,$2,$3,$4,'new epoch',$5,1,false)`, f.team, f.project.ID, f.appraiser.UserID, f.root.UserID, uuid.NewString())
	p05Must(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `SELECT contribution_set_appraiser($1,$2,$3,$4,'revoked',$5,2,true)`, f.team, f.project.ID, f.appraiser.UserID, f.root.UserID, uuid.NewString())
	p05Must(t, e)
	_, e = f.confirm(c, f.appraiser)
	p05Denied(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `SELECT contribution_set_appraiser($1,$2,$3,$4,'fresh grant',$5,3,false)`, f.team, f.project.ID, f.appraiser.UserID, f.root.UserID, uuid.NewString())
	p05Must(t, e)
	p05Must(t, f.ts.RemoveMember(f.ctx, f.root, f.team, f.appraiser.UserID))
	_, e = f.confirm(c, f.appraiser)
	p05Denied(t, e)
	if f.state(c) != "pending_review" || f.official(c) != 0 {
		t.Fatal("no independent person was silently bypassed")
	}
}
func TestContributionBeneficiaryProposerAndDeclaredConflict(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	f.grant(f.arbiter)
	f.grant(f.worker)
	c := f.create()
	_, e := f.db.Pool.Exec(f.ctx, `SELECT contribution_set_share($1,$2,$3,$4,$5,10000,'basis','statement','[{"kind":"self_statement","source":"member","reference":"declaration"}]',$6,$7,'manual allocation',$8)`, f.team, f.project.ID, c, f.latest(c), f.appraiser.UserID, f.root.UserID, f.rev(c), uuid.NewString())
	p05Must(t, e)
	p05Must(t, f.transition(c, "pending_review"))
	_, e = f.confirm(c, f.appraiser)
	p05Denied(t, e) // direct beneficiary
	_, e = f.confirm(c, f.worker)
	p05Denied(t, e) // accepted result author, not a beneficiary here
	_, e = f.db.Pool.Exec(f.ctx, `INSERT INTO contribution_conflicts(team_id,project_id,task_id,case_id,user_id,kind,details,recorded_by,request_id) VALUES($1,$2,$3,$4,$5,'financial_dependency','Declared direct interest',$6,$7)`, f.team, f.project.ID, f.task.ID, c, f.arbiter.UserID, f.root.UserID, uuid.NewString())
	p05Must(t, e)
	_, e = f.confirm(c, f.arbiter)
	p05Denied(t, e)
	independent := f.user("independent", true)
	f.grant(independent)
	_, e = f.confirm(c, independent)
	p05Must(t, e)
	var v uuid.UUID
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT contribution_revise($1,$2,$3,$4,$5,'proposer recusal',$6)`, f.team, f.project.ID, c, f.arbiter.UserID, f.rev(c), uuid.NewString()).Scan(&v))
	f.prepare(c)
	_, e = f.confirm(c, f.arbiter)
	p05Denied(t, e) // proposer as well as disclosed conflict
	if f.official(c) != 1 {
		t.Fatal("pending revision replaced confirmed shares")
	}
}
func TestContributionOverturnAndAtomicFailure(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	f.grant(f.arbiter)
	c := f.create()
	f.prepare(c)
	conf, e := f.confirm(c, f.appraiser)
	p05Must(t, e)
	first := f.latest(c)
	working := f.revise(c)
	f.prepare(c)
	d := f.dispute(c, conf)
	p05Denied(t, f.resolve(c, d, f.appraiser, "overturn"))
	p05Denied(t, f.resolve(c, d, f.worker, "overturn"))
	var before string
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT to_jsonb(c)::text FROM contribution_cases c WHERE id=$1`, c).Scan(&before))
	_, e = f.db.Pool.Exec(f.ctx, `CREATE FUNCTION p05_fail_audit() RETURNS trigger AS $$ BEGIN IF NEW.action='contribution.resolution_overturn' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql;CREATE TRIGGER p05_fail_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION p05_fail_audit()`)
	p05Must(t, e)
	p05Denied(t, f.resolve(c, d, f.arbiter, "overturn"))
	var after string
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT to_jsonb(c)::text FROM contribution_cases c WHERE id=$1`, c).Scan(&after))
	if before != after {
		t.Fatal("failed ruling changed authoritative pointers")
	}
	var n int
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_dispute_resolutions WHERE case_id=$1`, c).Scan(&n))
	if n != 0 {
		t.Fatal("partial ruling committed")
	}
	_, e = f.db.Pool.Exec(f.ctx, `DROP TRIGGER p05_fail_audit ON audit_events`)
	p05Must(t, e)
	p05Must(t, f.resolve(c, d, f.arbiter, "overturn"))
	if f.official(c) != 0 || f.latest(c) == first || f.latest(c) == working {
		t.Fatal("overturn did not atomically create successor draft")
	}
	var states []string
	rows, e := f.db.Pool.Query(f.ctx, `SELECT status FROM contribution_case_versions WHERE case_id=$1 ORDER BY version`, c)
	p05Must(t, e)
	for rows.Next() {
		var s string
		p05Must(t, rows.Scan(&s))
		states = append(states, s)
	}
	p05Must(t, rows.Err())
	rows.Close()
	if strings.Join(states, ",") != "overturned,abandoned,draft" {
		t.Fatal(states)
	}
	_, e = f.db.Pool.Exec(f.ctx, `DELETE FROM contribution_confirmations WHERE id=$1`, conf)
	p05Denied(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `UPDATE contribution_disputes SET reason='rewrite' WHERE id=$1`, d)
	p05Denied(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `INSERT INTO contribution_dispute_resolutions(team_id,project_id,task_id,case_id,dispute_id,decision,resolved_by,reason,request_id) VALUES($1,$2,$3,$4,$5,'uphold',$6,'contradiction',$7)`, f.team, f.project.ID, f.task.ID, c, d, f.arbiter.UserID, uuid.NewString())
	p05Denied(t, e)
	f.prepare(c)
	_, e = f.confirm(c, f.appraiser)
	p05Must(t, e)
	if f.official(c) != 1 {
		t.Fatal("new independently confirmed revision not effective")
	}
}
func TestContributionHistoricalMemberAndProxyDispute(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	f.grant(f.arbiter)
	p05Must(t, f.ts.RemoveMember(f.ctx, f.root, f.team, f.worker.UserID))
	c := f.create()
	f.prepare(c)
	conf, e := f.confirm(c, f.appraiser)
	p05Must(t, e)
	var state string
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT membership_snapshot->>$2 FROM contribution_confirmations WHERE id=$1`, conf, f.worker.UserID.String()).Scan(&state))
	if state != "historical" {
		t.Fatal(state)
	}
	_, e = f.db.Pool.Exec(f.ctx, `INSERT INTO contribution_disputes(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,raised_by,on_behalf_of,verification_method,verification_evidence_ref,verification_note,reason,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'telephone','free-text','claimed identity verified','proxy reason',$9)`, f.team, f.project.ID, f.task.ID, c, f.latest(c), conf, f.root.UserID, f.worker.UserID, uuid.NewString())
	p05Denied(t, e)
	login, e := f.as.Login(f.ctx, "worker", p05Password, "M2 identity proof", "127.0.0.1")
	p05Must(t, e)
	var evidence uuid.UUID
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `INSERT INTO contribution_attestations(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,purpose,subject_id,actor_id,statement,content_digest,proof_token,request_id) VALUES($1,$2,$3,$4,$5,$6,'dispute_request',$7,$7,'proxy reason',encode(sha256(convert_to('proxy reason','UTF8')),'hex'),$8,$9) RETURNING id`, f.team, f.project.ID, f.task.ID, c, f.latest(c), conf, f.worker.UserID, login.Token, uuid.NewString()).Scan(&evidence))
	var d uuid.UUID
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT contribution_raise_dispute($1,$2,$3,$4,$5,$6,'proxy reason',$7,$8,$9)`, f.team, f.project.ID, c, conf, f.root.UserID, f.rev(c), uuid.NewString(), f.worker.UserID, evidence).Scan(&d))
	if f.state(c) != "disputed" || f.official(c) != 0 {
		t.Fatal("proxy dispute bypassed official exclusion")
	}
	p05Must(t, f.resolve(c, d, f.arbiter, "uphold"))
	if f.official(c) != 1 {
		t.Fatal("uphold changed original shares")
	}
	notice := f.notice(c, conf, "authenticated_ack")
	p05Must(t, f.receipt(notice, login.Token))
	p05Must(t, f.archive())
}

func (f *contributionFixture) snapshot(tables []string) map[string]string {
	f.t.Helper()
	out := map[string]string{}
	for _, table := range tables {
		var raw string
		p05Must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text),'[]'::jsonb)::text FROM `+table+` x`).Scan(&raw))
		sum := sha256.Sum256([]byte(raw))
		out[table] = hex.EncodeToString(sum[:])
	}
	return out
}

var p05LegacyTables = []string{"users", "sessions", "teams", "team_members", "team_invitations", "projects", "project_members", "project_reviewers", "project_milestones", "tasks", "task_collaborators", "task_dependencies", "task_progress_history", "project_progress_snapshots", "task_submissions", "task_submission_versions", "task_reviews", "submission_attachments", "audit_events"}
var p05BusinessTables = []string{"contribution_appraisers", "contribution_cases", "contribution_case_versions", "contribution_shares", "contribution_share_adjustments", "contribution_confirmations", "contribution_conflicts", "contribution_disputes", "contribution_dispute_resolutions", "contribution_case_events", "contribution_notifications", "contribution_attestations", "audit_events"}

func TestContributionUpgradePreservesPhase04AndArchivedHistory(t *testing.T) {
	f := newContributionFixture(t, true)
	p05Must(t, f.archive())
	before := f.snapshot(p05LegacyTables)
	p05Must(t, database.Up(f.ctx, f.db.SQL, f.log))
	after := f.snapshot(p05LegacyTables)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("v7 changed preexisting Phase02-04 history")
	}
	var count int
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_cases`).Scan(&count))
	if count != 0 {
		t.Fatal("migration fabricated cases")
	}
	var id uuid.UUID
	e := f.db.Pool.QueryRow(f.ctx, `SELECT contribution_create_case($1,$2,$3,$4,$5,$6)`, f.team, f.project.ID, f.task.ID, f.review.ID, f.root.UserID, uuid.NewString()).Scan(&id)
	p05Denied(t, e)
	p05Must(t, database.Down(f.ctx, f.db.SQL, 1, f.log))
	if !reflect.DeepEqual(before, f.snapshot(p05LegacyTables)) {
		t.Fatal("empty Phase05 rollback changed Phase04 evidence")
	}
}
func TestContributionRollbackRefusesFormalEvidence(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	c := f.create()
	f.prepare(c)
	_, e := f.confirm(c, f.appraiser)
	p05Must(t, e)
	before := f.snapshot(p05BusinessTables)
	p05Denied(t, database.Down(f.ctx, f.db.SQL, 1, f.log))
	v, e := database.Version(f.ctx, f.db.SQL)
	p05Must(t, e)
	if v != 7 || !reflect.DeepEqual(before, f.snapshot(p05BusinessTables)) {
		t.Fatal("down damaged formal contribution evidence")
	}
}
func TestContributionConfirmationAndShareFailureAreAtomic(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	c := f.create()
	f.prepare(c)
	_, e := f.confirm(c, f.appraiser)
	p05Must(t, e)
	old := f.latest(c)
	f.revise(c)
	f.prepare(c)
	before := f.snapshot(p05BusinessTables)
	_, e = f.db.Pool.Exec(f.ctx, `CREATE FUNCTION p05_fail_confirm() RETURNS trigger AS $$ BEGIN IF NEW.action='contribution.confirmed' THEN RAISE EXCEPTION 'injected confirmation audit failure'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql;CREATE TRIGGER p05_fail_confirm BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION p05_fail_confirm()`)
	p05Must(t, e)
	_, e = f.confirm(c, f.appraiser)
	p05Denied(t, e)
	if !reflect.DeepEqual(before, f.snapshot(p05BusinessTables)) {
		t.Fatal("partial confirmation, supersede, pointer, history or audit committed")
	}
	_, e = f.db.Pool.Exec(f.ctx, `DROP TRIGGER p05_fail_confirm ON audit_events`)
	p05Must(t, e)
	_, e = f.confirm(c, f.appraiser)
	p05Must(t, e)
	var status string
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT status FROM contribution_case_versions WHERE id=$1`, old).Scan(&status))
	if status != "superseded" {
		t.Fatal(status)
	}
	f.revise(c)
	f.share(c, 9000)
	before = f.snapshot(p05BusinessTables)
	_, e = f.db.Pool.Exec(f.ctx, `CREATE FUNCTION p05_fail_share() RETURNS trigger AS $$ BEGIN IF NEW.action='contribution.share_adjusted' THEN RAISE EXCEPTION 'injected share audit failure'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql;CREATE TRIGGER p05_fail_share BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION p05_fail_share()`)
	p05Must(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `SELECT contribution_set_share($1,$2,$3,$4,$5,10000,'basis','statement','[]',$6,$7,'adjust',$8)`, f.team, f.project.ID, c, f.latest(c), f.worker.UserID, f.root.UserID, f.rev(c), uuid.NewString())
	p05Denied(t, e)
	if !reflect.DeepEqual(before, f.snapshot(p05BusinessTables)) {
		t.Fatal("partial share adjustment committed")
	}
}
func TestContributionDeliveryCannotBeForged(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	c := f.create()
	f.prepare(c)
	conf, e := f.confirm(c, f.appraiser)
	p05Must(t, e)
	notice := f.notice(c, conf, "authenticated_ack")
	login, e := f.as.Login(f.ctx, "appraiser", p05Password, "M2 proof", "127.0.0.1")
	p05Must(t, e)
	p05Denied(t, f.receipt(notice, login.Token))
	worker, e := f.as.Login(f.ctx, "worker", p05Password, "M2 proof", "127.0.0.1")
	p05Must(t, e)
	_, e = f.db.Pool.Exec(f.ctx, `INSERT INTO contribution_attestations(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,purpose,notice_id,subject_id,actor_id,content_digest,proof_token,request_id) VALUES($1,$2,$3,$4,$5,$6,'delivery',$7,$8,$8,repeat('a',64),$9,$10)`, f.team, f.project.ID, f.task.ID, c, f.latest(c), conf, notice, f.worker.UserID, worker.Token, uuid.NewString())
	p05Denied(t, e)
	_, e = f.as.RevokeUserSessions(f.ctx, f.root, f.worker.UserID)
	p05Must(t, e)
	p05Denied(t, f.receipt(notice, worker.Token))
	p05Denied(t, f.archive())
	worker, e = f.as.Login(f.ctx, "worker", p05Password, "M2 proof", "127.0.0.1")
	p05Must(t, e)
	p05Must(t, f.receipt(notice, worker.Token))
	var raw string
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT to_jsonb(a)::text FROM contribution_attestations a WHERE notice_id=$1`, notice).Scan(&raw))
	if strings.Contains(raw, worker.Token) {
		t.Fatal("raw authentication token persisted")
	}
	p05Must(t, f.archive())
}
func TestContributionMultipleDisputesPreserveOverturnHistory(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	f.grant(f.arbiter)
	c := f.create()
	f.prepare(c)
	conf, e := f.confirm(c, f.appraiser)
	p05Must(t, e)
	d1 := f.dispute(c, conf)
	d2 := f.dispute(c, conf)
	p05Must(t, f.resolve(c, d1, f.arbiter, "overturn"))
	if f.state(c) != "disputed" || f.official(c) != 0 {
		t.Fatal("remaining dispute disappeared")
	}
	successor := f.latest(c)
	p05Denied(t, f.transition(c, "pending_review"))
	p05Denied(t, f.resolve(c, d2, f.arbiter, "uphold"))
	p05Must(t, f.resolve(c, d2, f.arbiter, "overturn"))
	if f.latest(c) != successor {
		t.Fatal("repeated ruling duplicated successor")
	}
	var n int
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_confirmations WHERE case_id=$1`, c).Scan(&n))
	if n != 1 {
		t.Fatal("original confirmation was erased")
	}
}

// runLockedPair observes an actual PostgreSQL wait on the second connection
// before running the first action. No mock, artificial lock result or sleep race.
func (f *contributionFixture) runLockedPair(c *uuid.UUID, first, second func(pgx.Tx) error) (error, error) {
	f.t.Helper()
	one, e := f.db.Pool.Begin(f.ctx)
	p05Must(f.t, e)
	defer one.Rollback(f.ctx)
	two, e := f.db.Pool.Begin(f.ctx)
	p05Must(f.t, e)
	defer two.Rollback(f.ctx)
	_, e = one.Exec(f.ctx, `SELECT contribution_lock($1,$2,$3)`, f.team, f.project.ID, c)
	p05Must(f.t, e)
	done := make(chan error, 1)
	go func() {
		err := second(two)
		if err == nil {
			err = two.Commit(f.ctx)
		} else {
			_ = two.Rollback(f.ctx)
		}
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		var lock bool
		e = f.db.SQL.QueryRowContext(f.ctx, `SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, int64(two.Conn().PgConn().PID())).Scan(&lock)
		p05Must(f.t, e)
		if lock {
			waiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		f.t.Fatal("second PostgreSQL connection never entered a lock wait")
	}
	err1 := first(one)
	if err1 == nil {
		err1 = one.Commit(f.ctx)
	} else {
		_ = one.Rollback(f.ctx)
	}
	select {
	case err2 := <-done:
		var pg *pgconn.PgError
		for _, err := range []error{err1, err2} {
			if errors.As(err, &pg) && pg.Code == "40P01" {
				f.t.Fatal("contribution transaction deadlocked")
			}
		}
		return err1, err2
	case <-f.ctx.Done():
		f.t.Fatal("concurrent transaction did not complete")
		return nil, f.ctx.Err()
	}
}
func (f *contributionFixture) txConfirm(c uuid.UUID, expected int, a reqctx.Identity) func(pgx.Tx) error {
	return func(tx pgx.Tx) error {
		var id uuid.UUID
		return tx.QueryRow(f.ctx, `SELECT contribution_confirm($1,$2,$3,$4,$5,'concurrent confirmation',$6)`, f.team, f.project.ID, c, a.UserID, expected, uuid.NewString()).Scan(&id)
	}
}
func (f *contributionFixture) txDispute(c, conf uuid.UUID, expected int) func(pgx.Tx) error {
	return func(tx pgx.Tx) error {
		var id uuid.UUID
		return tx.QueryRow(f.ctx, `SELECT contribution_raise_dispute($1,$2,$3,$4,$5,$6,'concurrent dispute',$7,NULL,NULL)`, f.team, f.project.ID, c, conf, f.worker.UserID, expected, uuid.NewString()).Scan(&id)
	}
}
func (f *contributionFixture) txArchive() func(pgx.Tx) error {
	return func(tx pgx.Tx) error {
		_, e := tx.Exec(f.ctx, `UPDATE projects SET status='archived',archived_at=clock_timestamp(),version=version+1 WHERE team_id=$1 AND id=$2`, f.team, f.project.ID)
		return e
	}
}
func TestContributionConcurrentUniqueCreation(t *testing.T) {
	f := newContributionFixture(t, false)
	var id1, id2 uuid.UUID
	create := func(out *uuid.UUID) func(pgx.Tx) error {
		return func(tx pgx.Tx) error {
			return tx.QueryRow(f.ctx, `SELECT contribution_create_case($1,$2,$3,$4,$5,$6)`, f.team, f.project.ID, f.task.ID, f.review.ID, f.root.UserID, uuid.NewString()).Scan(out)
		}
	}
	a, b := f.runLockedPair(nil, create(&id1), create(&id2))
	p05Must(t, a)
	p05Must(t, b)
	if id1 != id2 {
		t.Fatal("duplicate case for approval")
	}
	var n int
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_case_versions WHERE case_id=$1`, id1).Scan(&n))
	if n != 1 {
		t.Fatal("duplicate initial version")
	}
}
func TestContributionConcurrentAdjustmentsAndDoubleConfirmation(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	f.grant(f.arbiter)
	c := f.create()
	f.share(c, 10000)
	expected, version := f.rev(c), f.latest(c)
	adjust := func(bps int) func(pgx.Tx) error {
		return func(tx pgx.Tx) error {
			_, e := tx.Exec(f.ctx, `SELECT contribution_set_share($1,$2,$3,$4,$5,$6,'basis','statement','[]',$7,$8,'concurrent adjustment',$9)`, f.team, f.project.ID, c, version, f.worker.UserID, bps, f.root.UserID, expected, uuid.NewString())
			return e
		}
	}
	a, b := f.runLockedPair(&c, adjust(8000), adjust(9000))
	p05Must(t, a)
	p05Denied(t, b)
	var bps int
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT share_bps FROM contribution_shares WHERE case_id=$1`, c).Scan(&bps))
	if bps != 8000 {
		t.Fatal("lost share update")
	}
	f.prepare(c)
	expected = f.rev(c)
	a, b = f.runLockedPair(&c, f.txConfirm(c, expected, f.appraiser), f.txConfirm(c, expected, f.arbiter))
	p05Must(t, a)
	p05Denied(t, b)
	var count int
	p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_confirmations WHERE case_id=$1`, c).Scan(&count))
	if count != 1 {
		t.Fatal("double formal confirmation")
	}
}
func TestContributionConcurrentConfirmationDisputeAndRuling(t *testing.T) {
	for _, firstDispute := range []bool{false, true} {
		t.Run(fmt.Sprint(firstDispute), func(t *testing.T) {
			f := newContributionFixture(t, false)
			f.grant(f.appraiser)
			f.grant(f.arbiter)
			c := f.create()
			f.prepare(c)
			conf, e := f.confirm(c, f.appraiser)
			p05Must(t, e)
			f.revise(c)
			f.prepare(c)
			expected := f.rev(c)
			first, second := f.txConfirm(c, expected, f.appraiser), f.txDispute(c, conf, expected)
			if firstDispute {
				first, second = second, first
			}
			a, b := f.runLockedPair(&c, first, second)
			p05Must(t, a)
			p05Denied(t, b)
			if firstDispute {
				if f.official(c) != 0 {
					t.Fatal("dispute leaked into official data")
				}
				var d uuid.UUID
				p05Must(t, f.db.Pool.QueryRow(f.ctx, `SELECT id FROM contribution_disputes WHERE case_id=$1`, c).Scan(&d))
				expected = f.rev(c)
				resolve := func(tx pgx.Tx) error {
					var id uuid.UUID
					return tx.QueryRow(f.ctx, `SELECT contribution_resolve($1,$2,$3,$4,$5,$6,'uphold','concurrent ruling',$7)`, f.team, f.project.ID, c, d, f.arbiter.UserID, expected, uuid.NewString()).Scan(&id)
				}
				a, b = f.runLockedPair(&c, resolve, f.txDispute(c, conf, expected))
				p05Must(t, a)
				p05Denied(t, b)
			}
		})
	}
}
func TestContributionConcurrentRevocationAndMemberExit(t *testing.T) {
	for _, kind := range []string{"revoke", "exit"} {
		for _, firstConfirm := range []bool{false, true} {
			t.Run(kind+fmt.Sprint(firstConfirm), func(t *testing.T) {
				f := newContributionFixture(t, false)
				f.grant(f.appraiser)
				c := f.create()
				f.prepare(c)
				expected := f.rev(c)
				change := func(tx pgx.Tx) error {
					if kind == "revoke" {
						_, e := tx.Exec(f.ctx, `SELECT contribution_set_appraiser($1,$2,$3,$4,'concurrent revoke',$5,1,true)`, f.team, f.project.ID, f.appraiser.UserID, f.root.UserID, uuid.NewString())
						return e
					}
					_, e := tx.Exec(f.ctx, `UPDATE team_members SET status='removed',removed_at=clock_timestamp(),updated_at=clock_timestamp() WHERE team_id=$1 AND user_id=$2`, f.team, f.appraiser.UserID)
					if e != nil {
						return e
					}
					_, e = tx.Exec(f.ctx, `INSERT INTO audit_events(actor_id,team_id,action,request_id) VALUES($1,$2,'team.member_removed',$3)`, f.root.UserID, f.team, uuid.NewString())
					return e
				}
				first, second := change, f.txConfirm(c, expected, f.appraiser)
				if firstConfirm {
					first, second = second, first
				}
				a, b := f.runLockedPair(&c, first, second)
				p05Must(t, a)
				if firstConfirm {
					p05Must(t, b)
					if f.official(c) != 1 {
						t.Fatal("valid historical confirmation invalidated by later departure")
					}
				} else {
					p05Denied(t, b)
					if f.official(c) != 0 || f.state(c) != "pending_review" {
						t.Fatal("revoked appraiser confirmed")
					}
				}
			})
		}
	}
}
func TestContributionConcurrentRevisionAndArchive(t *testing.T) {
	for _, archiveFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(archiveFirst), func(t *testing.T) {
			f := newContributionFixture(t, false)
			f.grant(f.appraiser)
			c := f.create()
			f.prepare(c)
			conf, e := f.confirm(c, f.appraiser)
			p05Must(t, e)
			notice := f.notice(c, conf, "authenticated_ack")
			login, e := f.as.Login(f.ctx, "worker", p05Password, "M2 archive race", "127.0.0.1")
			p05Must(t, e)
			p05Must(t, f.receipt(notice, login.Token))
			expected := f.rev(c)
			revise := func(tx pgx.Tx) error {
				var id uuid.UUID
				return tx.QueryRow(f.ctx, `SELECT contribution_revise($1,$2,$3,$4,$5,'concurrent revision',$6)`, f.team, f.project.ID, c, f.root.UserID, expected, uuid.NewString()).Scan(&id)
			}
			first, second := revise, f.txArchive()
			if archiveFirst {
				first, second = second, first
			}
			a, b := f.runLockedPair(&c, first, second)
			p05Must(t, a)
			p05Denied(t, b)
		})
	}
	// Confirmation can finish first, but old-version delivery must never satisfy
	// the replacement result's archive gate.
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	c := f.create()
	f.prepare(c)
	conf, e := f.confirm(c, f.appraiser)
	p05Must(t, e)
	notice := f.notice(c, conf, "authenticated_ack")
	login, e := f.as.Login(f.ctx, "worker", p05Password, "M2 archive race", "127.0.0.1")
	p05Must(t, e)
	p05Must(t, f.receipt(notice, login.Token))
	f.revise(c)
	f.prepare(c)
	a, b := f.runLockedPair(&c, f.txConfirm(c, f.rev(c), f.appraiser), f.txArchive())
	p05Must(t, a)
	p05Denied(t, b)
}

func TestContributionRejectsStaleAuthorizationSnapshots(t *testing.T) {
	f := newContributionFixture(t, false)
	f.grant(f.appraiser)
	c := f.create()
	f.prepare(c)
	expected := f.rev(c)
	tx, e := f.db.Pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	p05Must(t, e)
	defer tx.Rollback(f.ctx)
	var active bool
	p05Must(t, tx.QueryRow(f.ctx, `SELECT contribution_appraiser($1,$2,$3)`, f.team, f.project.ID, f.appraiser.UserID).Scan(&active))
	if !active {
		t.Fatal("fixture lacks initial authorization")
	}
	_, e = f.db.Pool.Exec(f.ctx, `SELECT contribution_set_appraiser($1,$2,$3,$4,'revoke before confirmation',$5,1,true)`, f.team, f.project.ID, f.appraiser.UserID, f.root.UserID, uuid.NewString())
	p05Must(t, e)
	err := f.txConfirm(c, expected, f.appraiser)(tx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "25001" {
		t.Fatalf("stale authorization snapshot must be explicitly rejected: %v", err)
	}
	p05Must(t, tx.Rollback(f.ctx))
	tx, e = f.db.Pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	p05Must(t, e)
	defer tx.Rollback(f.ctx)
	err = f.txArchive()(tx)
	if !errors.As(err, &pgErr) || pgErr.Code != "25001" {
		t.Fatalf("archive cannot use a stale settlement snapshot: %v", err)
	}
	p05Must(t, tx.Rollback(f.ctx))
	if f.official(c) != 0 || f.state(c) != "pending_review" {
		t.Fatal("old snapshot bypassed permission revocation")
	}
}
