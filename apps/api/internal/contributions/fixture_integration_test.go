//go:build integration

package contributions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/contributions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpapi"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/submissions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/system"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/tasks"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

const password = "contribution-integration-password"

type fixture struct {
	t          *testing.T
	ctx        context.Context
	db         *testdb.Fixture
	logs       bytes.Buffer
	as         *auth.Service
	ts         *teams.Service
	ps         *projects.Service
	tasks      *tasks.Service
	sub        *submissions.Service
	svc        *contributions.Service
	handler    http.Handler
	cookies    map[string]*http.Cookie
	actors     map[string]reqctx.Identity
	team       uuid.UUID
	project    projects.Project
	task       tasks.Task
	review     submissions.Review
	submission submissions.Version
}

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func status(t *testing.T, e error, want int) {
	t.Helper()
	var db interface{ SQLState() string }
	if errors.As(e, &db) && db.SQLState() == "40P01" {
		t.Fatal("database deadlock")
	}
	var a *httpx.APIError
	if !errors.As(e, &a) || a.Status != want {
		t.Fatalf("want status %d, got %v", want, e)
	}
}
func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, db: testdb.New(t, os.Getenv), cookies: map[string]*http.Cookie{}, actors: map[string]reqctx.Identity{}}
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(&f.logs, nil))
	must(t, database.Up(f.ctx, f.db.SQL, logger))
	ar := audit.NewRepository(f.db.Pool)
	f.as = auth.NewService(auth.NewRepository(f.db.Pool), ar, auth.NewLoginLimiter(1000, time.Minute), time.Hour, logger)
	tr := teams.NewRepository(f.db.Pool)
	f.ts = teams.NewService(tr, f.as, ar, logger)
	f.ps = projects.NewService(projects.NewRepository(f.db.Pool), tr, ar, logger)
	f.tasks = tasks.NewService(tasks.NewRepository(), f.ps)
	f.sub = submissions.NewService(submissions.NewRepository(f.db.Pool), f.ps, f.tasks)
	store, e := submissions.OpenStore(t.TempDir())
	must(t, e)
	t.Cleanup(func() { store.Close() })
	f.sub.SetStore(store)
	f.svc = contributions.NewService(contributions.NewRepository(f.db.Pool), f.ps)
	cfg := config.Config{Environment: config.EnvironmentTest, SessionTTL: time.Hour}
	f.handler = httpapi.New(cfg, logger, system.NewRepository(f.db.Pool), f.as, auth.NewHandler(f.as, false, time.Hour, logger), teams.NewHandler(f.ts, logger), projects.NewHandler(f.ps, logger), tasks.NewHandler(f.tasks, logger), submissions.NewHandler(f.sub, logger), contributions.NewHandler(f.svc, logger)).Handler()
	_, e = f.as.BootstrapRoot(f.ctx, "root", password)
	must(t, e)
	f.login("root")
	tm, e := f.ts.CreateTeam(f.ctx, f.actors["root"], "M3 team", "")
	must(t, e)
	f.team = tm.ID
	for _, name := range []string{"lead", "worker", "appraiser", "arbiter", "member", "outsider"} {
		_, e = f.as.CreateUser(f.ctx, f.actors["root"], name, name, password)
		must(t, e)
		f.login(name)
		if name != "outsider" {
			inv, e := f.ts.Invite(f.ctx, f.actors["root"], f.team, name, teams.RoleMember, 1)
			must(t, e)
			_, e = f.ts.AcceptInvitation(f.ctx, f.actors[name], inv.ID)
			must(t, e)
		}
	}
	f.project, e = f.ps.Create(f.ctx, f.actors["root"], f.team, projects.CreateInput{ProjectInput: projects.ProjectInput{Name: "M3 project"}, LeadID: f.actors["lead"].UserID})
	must(t, e)
	for _, name := range []string{"worker", "appraiser", "arbiter", "member"} {
		_, e = f.ps.ChangeMember(f.ctx, f.actors["lead"], f.team, f.project.ID, f.actors[name].UserID, true)
		must(t, e)
	}
	// A real Team owner/ROOT, but no project relationship: platform privilege must
	// not become implicit contribution read or appraisal authority.
	_, e = f.ps.ChangeMember(f.ctx, f.actors["lead"], f.team, f.project.ID, f.actors["root"].UserID, false)
	must(t, e)
	f.task, e = f.tasks.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, tasks.CreateInput{TaskFields: tasks.TaskFields{Title: "Real accepted work", OwnerID: f.actors["worker"].UserID}, Weight: 7})
	must(t, e)
	f.task, e = f.tasks.Transition(f.ctx, f.actors["worker"], f.team, f.project.ID, f.task.ID, tasks.InProgress, "", f.task.Version)
	must(t, e)
	file, e := f.sub.Upload(f.ctx, f.actors["worker"], f.team, f.project.ID, f.task.ID, uuid.New(), "evidence.txt", strings.NewReader("immutable actual Phase04 evidence"))
	must(t, e)
	f.submission, e = f.sub.Submit(f.ctx, f.actors["worker"], f.team, f.project.ID, f.task.ID, submissions.SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: f.task.Version, Summary: "Accepted source", AttachmentIDs: []uuid.UUID{file.ID}})
	must(t, e)
	f.task, e = f.tasks.Get(f.ctx, f.actors["lead"], f.team, f.project.ID, f.task.ID)
	must(t, e)
	f.review, e = f.sub.Review(f.ctx, f.actors["lead"], f.team, f.project.ID, f.task.ID, f.submission.ID, submissions.ReviewInput{ExpectedVersion: f.task.Version, Decision: "approve", Comment: "Approve immutable evidence"})
	must(t, e)
	return f
}
func (f *fixture) do(method, path string, body any, cookie *http.Cookie, csrf bool) *httptest.ResponseRecorder {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		must(f.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf {
		req.Header.Set(auth.CSRFHeader, "1")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, req)
	for _, c := range f.cookies {
		if strings.Contains(rr.Body.String(), c.Value) {
			f.t.Fatal("HTTP JSON exposed a session credential")
		}
	}
	return rr
}
func (f *fixture) login(name string) {
	f.t.Helper()
	r := f.do("POST", "/api/v1/auth/login", map[string]string{"username": name, "password": password}, nil, true)
	if r.Code != 200 {
		f.t.Fatalf("login: %d", r.Code)
	}
	cs := r.Result().Cookies()
	if len(cs) != 1 || !cs[0].HttpOnly {
		f.t.Fatal("missing HttpOnly login cookie")
	}
	f.cookies[name] = cs[0]
	a, e := f.as.Authenticate(f.ctx, cs[0].Value)
	must(f.t, e)
	f.actors[name] = a
}
func (f *fixture) base() string {
	return fmt.Sprintf("/api/v1/teams/%s/projects/%s", f.team, f.project.ID)
}
func (f *fixture) path(c uuid.UUID, suffix string) string {
	return f.base() + "/contributions/" + c.String() + suffix
}
func data[T any](t *testing.T, r *httptest.ResponseRecorder, want int) T {
	t.Helper()
	if r.Code != want {
		t.Fatalf("HTTP status=%d want=%d body=%s", r.Code, want, r.Body.String())
	}
	var out struct {
		Data T `json:"data"`
	}
	must(t, json.Unmarshal(r.Body.Bytes(), &out))
	return out.Data
}
func (f *fixture) http(method, path, who string, body any, want int) contributions.CommandResult {
	f.t.Helper()
	return data[contributions.CommandResult](f.t, f.do(method, path, body, f.cookies[who], true), want)
}
func (f *fixture) create() uuid.UUID {
	f.t.Helper()
	r := f.http("POST", f.base()+"/contributions", "lead", contributions.CreateInput{TaskID: f.task.ID, ReviewID: f.review.ID, ClientRequestID: uuid.New(), Reason: "Allocate accepted delivery"}, 201)
	return r.ID
}
func (f *fixture) current(c uuid.UUID) contributions.VersionCommand {
	f.t.Helper()
	var x contributions.VersionCommand
	must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT revision,latest_version_id FROM contribution_cases WHERE id=$1`, c).Scan(&x.ExpectedVersion, &x.CaseVersionID))
	x.ClientRequestID = uuid.New()
	x.Reason = "Explicit manual governance action"
	return x
}
func (f *fixture) grant(who string) {
	f.t.Helper()
	f.http("POST", f.base()+"/contribution-appraisers", "lead", contributions.AppraiserInput{Command: contributions.Command{ExpectedVersion: 0, ClientRequestID: uuid.New(), Reason: "Independent appraisal authority"}, UserID: f.actors[who].UserID}, 200)
}
func (f *fixture) share(c uuid.UUID, bps int) {
	f.t.Helper()
	f.http("PUT", f.path(c, "/shares/"+f.actors["worker"].UserID.String()), "lead", contributions.ShareInput{VersionCommand: f.current(c), ShareBPS: bps, Basis: "Manual assessment of objective work", Statement: "Subject statement", EvidenceRefs: []contributions.EvidenceReference{{Kind: "objective_fact", Source: "accepted_submission", Reference: f.submission.ID.String()}}}, 200)
}
func (f *fixture) prepare(c uuid.UUID) {
	f.share(c, 10000)
	f.http("POST", f.path(c, "/submit"), "lead", f.current(c), 200)
}
func (f *fixture) confirm(c uuid.UUID) uuid.UUID {
	return f.http("POST", f.path(c, "/confirm"), "appraiser", f.current(c), 200).ID
}
func (f *fixture) detail(c uuid.UUID) contributions.CaseDetail {
	return data[contributions.CaseDetail](f.t, f.do("GET", f.path(c, ""), nil, f.cookies["lead"], false), 200)
}
func (f *fixture) official(c uuid.UUID) int {
	var n int
	must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_effective_shares WHERE case_id=$1`, c).Scan(&n))
	return n
}
func (f *fixture) notice(c, confirmation uuid.UUID) uuid.UUID {
	x := f.current(c)
	return f.http("POST", f.path(c, "/notices"), "lead", contributions.NoticeInput{VersionCommand: x, ConfirmationID: confirmation, RecipientID: f.actors["worker"].UserID}, 201).ID
}
func (f *fixture) receiptInput(c, confirmation uuid.UUID) contributions.ReceiptInput {
	x := f.current(c)
	d := f.detail(c)
	return contributions.ReceiptInput{ExpectedVersion: x.ExpectedVersion, ClientRequestID: uuid.New(), CaseVersionID: *d.Case.EffectiveConfirmedVersionID, ConfirmationID: confirmation, ContentDigest: d.Effective.Confirmation.VersionDigest, Received: true}
}
