//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	"github.com/YEXIAONAN/Nexora/apps/api/internal/bonusresults"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/bonusrounds"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpapi"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/system"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

const bonusPassword = "bonus-http-integration-password"

type bonusHTTPFixture struct {
	t            *testing.T
	ctx          context.Context
	db           *testdb.Fixture
	handler      http.Handler
	team         uuid.UUID
	project      uuid.UUID
	otherProject uuid.UUID
	rounds       []uuid.UUID
	cookies      map[string]*http.Cookie
}

func newBonusHTTPFixture(t *testing.T) *bonusHTTPFixture {
	t.Helper()
	f := &bonusHTTPFixture{t: t, db: testdb.New(t, os.Getenv), cookies: map[string]*http.Cookie{}}
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Up(f.ctx, f.db.SQL, logger); err != nil {
		t.Fatal(err)
	}
	as := auth.NewService(auth.NewRepository(f.db.Pool), audit.NewRepository(f.db.Pool), auth.NewLoginLimiter(1000, time.Minute), time.Hour, logger)
	root, err := as.BootstrapRoot(f.ctx, "root", bonusPassword)
	if err != nil {
		t.Fatal(err)
	}
	rootIdentity := reqctx.Identity{UserID: root.ID, SystemRoot: true}
	for _, name := range []string{"member", "outsider", "other"} {
		if _, err := as.CreateUser(f.ctx, rootIdentity, name, name, bonusPassword); err != nil {
			t.Fatal(err)
		}
	}
	users := map[string]uuid.UUID{}
	for _, name := range []string{"root", "member", "outsider", "other"} {
		result, err := as.Login(f.ctx, name, bonusPassword, "test", "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		users[name] = result.User.ID
		f.cookies[name] = &http.Cookie{Name: auth.SessionCookieName, Value: result.Token}
	}
	f.team, f.project = uuid.New(), uuid.New()
	otherTeam := uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.db.Pool.Exec(f.ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO teams(id,name,created_by) VALUES($1,'Bonus HTTP team',$2),($3,'Other team',$2)", f.team, users["root"], otherTeam)
	exec("INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'member'),($1,$4,'member'),($5,$6,'owner')",
		f.team, users["root"], users["member"], users["outsider"], otherTeam, users["other"])
	exec("INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Bonus HTTP project',$3,$4)", f.project, f.team, users["member"], users["root"])
	exec("INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3)", f.team, f.project, users["member"])
	exec("UPDATE projects SET status='archived',archived_at=clock_timestamp(),version=version+1 WHERE id=$1", f.project)
	f.rounds = []uuid.UUID{
		f.insertRound(users["root"], "school-2026", "School prize", "school"),
		f.insertRound(users["root"], "college-2026", "College prize", "college"),
	}
	f.otherProject = uuid.New()
	exec("INSERT INTO projects(id,team_id,name,lead_id,created_by) VALUES($1,$2,'Another project',$3,$4)", f.otherProject, f.team, users["member"], users["root"])
	exec("INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3)", f.team, f.otherProject, users["member"])

	cfg := config.Config{Environment: config.EnvironmentTest, SessionTTL: time.Hour}
	f.handler = httpapi.New(cfg, logger, system.NewRepository(f.db.Pool), as, auth.NewHandler(as, false, time.Hour, logger), nil,
		bonusresults.NewHandler(bonusresults.NewService(bonusresults.NewRepository(f.db.Pool)), logger),
		bonusrounds.NewHandler(bonusrounds.NewService(bonusrounds.NewRepository(f.db.Pool)), logger),
	).Handler()
	return f
}

func (f *bonusHTTPFixture) insertRound(owner uuid.UUID, key, label, source string) uuid.UUID {
	f.t.Helper()
	round, version := uuid.New(), uuid.New()
	tx, err := f.db.Pool.Begin(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	defer tx.Rollback(f.ctx) //nolint:errcheck // rollback after commit is harmless
	if _, err := tx.Exec(f.ctx, `INSERT INTO bonus_rounds(id,team_id,project_id,funding_key,funding_label,funding_source,created_by,latest_version_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, round, f.team, f.project, key, label, source, owner, version); err != nil {
		f.t.Fatal(err)
	}
	if _, err := tx.Exec(f.ctx, `INSERT INTO bonus_round_versions(id,team_id,project_id,round_id,version,total_budget_minor,plan_unallocated_minor,proposed_by,rationale,client_request_id)
		VALUES($1,$2,$3,$4,1,100,100,$5,'draft HTTP metadata only',$6)`, version, f.team, f.project, round, owner, uuid.New()); err != nil {
		f.t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	return round
}

func (f *bonusHTTPFixture) do(method, path, who string) *httptest.ResponseRecorder {
	f.t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(nil))
	if who != "" {
		request.AddCookie(f.cookies[who])
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	for _, cookie := range f.cookies {
		if strings.Contains(response.Body.String(), cookie.Value) {
			f.t.Fatal("HTTP JSON exposed a session credential")
		}
	}
	return response
}

func (f *bonusHTTPFixture) roundsPath() string {
	return "/api/v1/teams/" + f.team.String() + "/projects/" + f.project.String() + "/bonus-rounds"
}

func decodeData(t *testing.T, response *httptest.ResponseRecorder, want int, out any) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status = %d, want %d: %s", response.Code, want, response.Body.String())
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Body.Bytes()))
	decoder.DisallowUnknownFields()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if out == nil {
		return
	}
	strict := json.NewDecoder(bytes.NewReader(envelope.Data))
	strict.DisallowUnknownFields()
	if err := strict.Decode(out); err != nil {
		t.Fatalf("decode data: %v", err)
	}
}

type bonusPage struct {
	Items   []json.RawMessage `json:"items"`
	HasMore bool              `json:"has_more"`
}

type bonusRoundItem struct {
	ID                        uuid.UUID     `json:"id"`
	ProjectID                 uuid.UUID     `json:"project_id"`
	FundingKey                string        `json:"funding_key"`
	FundingLabel              string        `json:"funding_label"`
	FundingSource             string        `json:"funding_source"`
	Status                    string        `json:"status"`
	Revision                  int           `json:"revision"`
	CreatedAt                 time.Time     `json:"created_at"`
	LatestVersion             *bonusVersion `json:"latest_version"`
	EffectiveConfirmedVersion *bonusVersion `json:"effective_confirmed_version"`
	AmountsVisible            bool          `json:"amounts_visible"`
}

type bonusVersion struct {
	Version     int        `json:"version"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	SubmittedAt *time.Time `json:"submitted_at"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
	Effective   bool       `json:"effective"`
}

type bonusRoundDetail struct {
	bonusRoundItem
	Versions []bonusVersion `json:"versions"`
}

func TestBonusHTTPAuthenticationAndFailClosedVisibility(t *testing.T) {
	f := newBonusHTTPFixture(t)
	mine := "/api/v1/teams/" + f.team.String() + "/bonus-results/mine"
	rounds := f.roundsPath()

	for _, tc := range []struct{ name, path string }{
		{"mine", mine},
		{"round list", rounds},
		{"round detail", rounds + "/" + f.rounds[0].String()},
	} {
		if response := f.do("GET", tc.path, ""); response.Code != http.StatusUnauthorized {
			t.Fatalf("%s without a cookie: status = %d, want 401", tc.name, response.Code)
		}
	}

	// An unknown cookie is rejected by the real session middleware.
	request := httptest.NewRequest("GET", mine, nil)
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "not-a-real-session"})
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid session: status = %d, want 401", response.Code)
	}

	// The subject-only endpoint returns a stable empty collection: M2 blocks
	// formal confirmations, so no fabricated amount can appear.
	var empty bonusPage
	response = f.do("GET", mine, "member")
	decodeData(t, response, http.StatusOK, &empty)
	if empty.Items == nil || len(empty.Items) != 0 || empty.HasMore {
		t.Fatalf("expected an explicit empty personal result set: %+v", empty)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	if response.Header().Get("X-Request-Id") == "" {
		t.Fatal("response omitted X-Request-Id")
	}

	// Team member without project membership and ROOT without membership are
	// both hidden, and neither leaks whether the team/project exists.
	for _, who := range []string{"outsider", "root"} {
		if response := f.do("GET", mine, who); response.Code != http.StatusNotFound {
			t.Fatalf("%s mine: status = %d, want 404", who, response.Code)
		}
		if response := f.do("GET", rounds, who); response.Code != http.StatusNotFound {
			t.Fatalf("%s round list: status = %d, want 404", who, response.Code)
		}
		if response := f.do("GET", rounds+"/"+f.rounds[0].String(), who); response.Code != http.StatusNotFound {
			t.Fatalf("%s round detail: status = %d, want 404", who, response.Code)
		}
	}
	// A different Team is not readable.
	if response := f.do("GET", "/api/v1/teams/"+uuid.NewString()+"/bonus-results/mine", "member"); response.Code != http.StatusNotFound {
		t.Fatalf("cross-team mine: status = %d, want 404", response.Code)
	}

	// Revoking the server-side session takes effect before any business query.
	var memberSession uuid.UUID
	if err := f.db.Pool.QueryRow(f.ctx, `SELECT id FROM sessions WHERE user_id=(SELECT id FROM users WHERE username='member') ORDER BY created_at DESC LIMIT 1`).Scan(&memberSession); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Pool.Exec(f.ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=$1`, memberSession); err != nil {
		t.Fatal(err)
	}
	if response := f.do("GET", mine, "member"); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session: status = %d, want 401", response.Code)
	}
}

func TestBonusHTTPRoundMetadataContract(t *testing.T) {
	f := newBonusHTTPFixture(t)
	rounds := f.roundsPath()

	var page bonusPage
	response := f.do("GET", rounds, "member")
	decodeData(t, response, http.StatusOK, &page)
	if len(page.Items) != 2 || page.HasMore {
		t.Fatalf("rounds = %d, want 2", len(page.Items))
	}
	for _, raw := range page.Items {
		var item bonusRoundItem
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatal(err)
		}
		if item.Status != bonusrounds.StatusDraft || item.AmountsVisible || item.EffectiveConfirmedVersion != nil {
			t.Fatalf("unexpected summary: %+v", item)
		}
		if item.LatestVersion == nil || item.LatestVersion.Version != 1 || item.LatestVersion.Status != "draft" {
			t.Fatalf("latest version missing: %+v", item)
		}
		if item.ProjectID != f.project {
			t.Fatal("round leaked a foreign project")
		}
	}
	// Draft money never appears in a project-member response.
	body := response.Body.String()
	for _, forbidden := range []string{"total_budget_minor", "plan_unallocated_minor", "source_budget_minor", "amount_minor", "shares_snapshot"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("round list exposed %s", forbidden)
		}
	}

	var detail bonusRoundDetail
	response = f.do("GET", rounds+"/"+f.rounds[0].String(), "member")
	decodeData(t, response, http.StatusOK, &detail)
	if detail.FundingSource != "school" || len(detail.Versions) != 1 || detail.Versions[0].Effective {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.AmountsVisible {
		t.Fatal("detail exposed amounts")
	}

	// A round from another project is hidden even inside the same Team.
	other := "/api/v1/teams/" + f.team.String() + "/projects/" + f.otherProject.String() + "/bonus-rounds/" + f.rounds[0].String()
	if response := f.do("GET", other, "member"); response.Code != http.StatusNotFound {
		t.Fatalf("cross-project round: status = %d, want 404", response.Code)
	}
	var otherPage bonusPage
	response = f.do("GET", "/api/v1/teams/"+f.team.String()+"/projects/"+f.otherProject.String()+"/bonus-rounds", "member")
	decodeData(t, response, http.StatusOK, &otherPage)
	if len(otherPage.Items) != 0 || otherPage.Items == nil {
		t.Fatalf("other project must expose an explicit empty set: %+v", otherPage)
	}
}

func TestBonusRoutesAreAbsentOnSchemaV8(t *testing.T) {
	f := &bonusHTTPFixture{t: t, db: testdb.New(t, os.Getenv), cookies: map[string]*http.Cookie{}}
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Up(f.ctx, f.db.SQL, logger); err != nil {
		t.Fatal(err)
	}
	// Roll the bonus foundation back so this fixture runs on the released
	// Alpha P0 v8 schema.
	if err := database.Down(f.ctx, f.db.SQL, 1, logger); err != nil {
		t.Fatal(err)
	}
	version, err := database.Version(f.ctx, f.db.SQL)
	if err != nil || version != 8 {
		t.Fatalf("schema version = %d, err = %v; want 8", version, err)
	}
	as := auth.NewService(auth.NewRepository(f.db.Pool), audit.NewRepository(f.db.Pool), auth.NewLoginLimiter(1000, time.Minute), time.Hour, logger)
	if _, err := as.BootstrapRoot(f.ctx, "root", bonusPassword); err != nil {
		t.Fatal(err)
	}
	result, err := as.Login(f.ctx, "root", bonusPassword, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: auth.SessionCookieName, Value: result.Token}
	cfg := config.Config{Environment: config.EnvironmentTest, SessionTTL: time.Hour}
	var modules []httpapi.RouteModule
	if database.SupportsBonusSchema(version) {
		modules = append(modules,
			bonusresults.NewHandler(bonusresults.NewService(bonusresults.NewRepository(f.db.Pool)), logger),
			bonusrounds.NewHandler(bonusrounds.NewService(bonusrounds.NewRepository(f.db.Pool)), logger),
		)
	}
	handler := httpapi.New(cfg, logger, system.NewRepository(f.db.Pool), as, auth.NewHandler(as, false, time.Hour, logger), nil, modules...).Handler()
	// Alpha P0 diagnostics and public metadata keep working on a v8 database.
	for _, path := range []string{"/readyz", "/api/v1/meta"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("schema v8 baseline path %s: status = %d, want 200", path, response.Code)
		}
	}
	for _, path := range []string{
		"/api/v1/teams/" + uuid.NewString() + "/bonus-results/mine",
		"/api/v1/teams/" + uuid.NewString() + "/projects/" + uuid.NewString() + "/bonus-rounds",
	} {
		request := httptest.NewRequest("GET", path, nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("schema v8 path %s: status = %d, want a safe 404 (never 500)", path, response.Code)
		}
	}
}
