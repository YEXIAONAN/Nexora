//go:build integration

package httpapi

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

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/submissions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/system"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/tasks"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

const httpRootPassword = "root-bootstrap-password-1"

func newAuthAPIFixture(t *testing.T, env config.Environment, maxFailures int) *httpAPIFixture {
	t.Helper()
	fixture := testdb.New(t, os.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Up(ctx, fixture.SQL, logger); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	cfg := config.Config{
		Environment:      env,
		SessionTTL:       time.Hour,
		LoginMaxFailures: maxFailures,
		LoginWindow:      15 * time.Minute,
	}
	auditRepo := audit.NewRepository(fixture.Pool)
	svc := auth.NewService(auth.NewRepository(fixture.Pool), auditRepo, auth.NewLoginLimiter(cfg.LoginMaxFailures, cfg.LoginWindow), cfg.SessionTTL, logger)
	handler := auth.NewHandler(svc, cfg.CookieSecure(), cfg.SessionTTL, logger)
	teamsSvc := teams.NewService(teams.NewRepository(fixture.Pool), svc, auditRepo, logger)
	teamsHandler := teams.NewHandler(teamsSvc, logger)
	projectSvc := projects.NewService(projects.NewRepository(fixture.Pool), teams.NewRepository(fixture.Pool), auditRepo, logger)
	taskSvc := tasks.NewService(tasks.NewRepository(), projectSvc)
	store, e := submissions.OpenStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	submissionSvc := submissions.NewService(submissions.NewRepository(fixture.Pool), projectSvc, taskSvc)
	submissionSvc.SetStore(store)
	server := New(cfg, logger, system.NewRepository(fixture.Pool), svc, handler, teamsHandler, projects.NewHandler(projectSvc, logger), tasks.NewHandler(taskSvc, logger), submissions.NewHandler(submissionSvc, logger))
	return &httpAPIFixture{handler: server.Handler(), svc: svc, teamsSvc: teamsSvc, t: t, ctx: ctx}
}

type httpAPIFixture struct {
	handler  http.Handler
	svc      *auth.Service
	teamsSvc *teams.Service
	t        *testing.T
	ctx      context.Context
}

func (f *httpAPIFixture) do(method, path string, body any, cookie *http.Cookie, csrf bool) *httptest.ResponseRecorder {
	f.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf {
		request.Header.Set(auth.CSRFHeader, "1")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func (f *httpAPIFixture) bootstrapRoot(username string) {
	f.t.Helper()
	if _, err := f.svc.BootstrapRoot(f.ctx, username, httpRootPassword); err != nil {
		f.t.Fatalf("bootstrap: %v", err)
	}
}

func (f *httpAPIFixture) login(username, password string) (*http.Cookie, map[string]any) {
	f.t.Helper()
	recorder := f.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": username, "password": password}, nil, true)
	if recorder.Code != http.StatusOK {
		f.t.Fatalf("login status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		f.t.Fatal(err)
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName {
			return cookie, payload.Data
		}
	}
	f.t.Fatal("login did not set the session cookie")
	return nil, nil
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v (%s)", err, recorder.Body.String())
	}
	if envelope.Error.RequestID == "" {
		t.Fatal("error response must carry request_id")
	}
	return envelope.Error.Code
}

func TestLoginCookieAttributes(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	f.bootstrapRoot("waiting")
	cookie, data := f.login("waiting", httpRootPassword)
	if !cookie.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if cookie.Secure {
		t.Error("test environment cookie must not set Secure over HTTP")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Path != "/" || cookie.Domain != "" {
		t.Errorf("cookie must be host-only with Path=/: %+v", cookie)
	}
	if cookie.MaxAge != 3600 {
		t.Errorf("MaxAge = %d, want session TTL seconds", cookie.MaxAge)
	}
	user := data["user"].(map[string]any)
	if user["system_root"] != true {
		t.Errorf("login response user = %v", user)
	}
}

func TestProductionCookieForcesSecure(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentProduction, 10)
	f.bootstrapRoot("waiting")
	cookie, _ := f.login("waiting", httpRootPassword)
	if !cookie.Secure {
		t.Fatal("production session cookie must set Secure")
	}
}

func TestProtectedEndpointRequiresSession(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	recorder := f.do(http.MethodGet, "/api/v1/auth/me", nil, nil, false)
	if recorder.Code != http.StatusUnauthorized || errorCode(t, recorder) != "unauthorized" {
		t.Fatalf("status = %d, want 401 unauthorized: %s", recorder.Code, recorder.Body.String())
	}
}

func TestMeLogoutFlow(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	f.bootstrapRoot("waiting")
	cookie, _ := f.login("waiting", httpRootPassword)

	me := f.do(http.MethodGet, "/api/v1/auth/me", nil, cookie, false)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d: %s", me.Code, me.Body.String())
	}

	// Security scenario 7: mutating request without the CSRF header is 403.
	noCSRF := f.do(http.MethodDelete, "/api/v1/auth/session", nil, cookie, false)
	if noCSRF.Code != http.StatusForbidden || errorCode(t, noCSRF) != "forbidden" {
		t.Fatalf("logout without CSRF header = %d, want 403", noCSRF.Code)
	}

	logout := f.do(http.MethodDelete, "/api/v1/auth/session", nil, cookie, true)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d: %s", logout.Code, logout.Body.String())
	}
	cleared := false
	for _, c := range logout.Result().Cookies() {
		if c.Name == auth.SessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout must clear the session cookie")
	}

	after := f.do(http.MethodGet, "/api/v1/auth/me", nil, cookie, false)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session accepted: %d", after.Code)
	}
}

func TestLoginRateLimitHTTP(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 2)
	f.bootstrapRoot("waiting")
	for range 2 {
		recorder := f.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "waiting", "password": "wrong-password-123"}, nil, true)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("bad login status = %d", recorder.Code)
		}
	}
	recorder := f.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "waiting", "password": httpRootPassword}, nil, true)
	if recorder.Code != http.StatusTooManyRequests || errorCode(t, recorder) != "rate_limited" {
		t.Fatalf("limited login = %d, want 429 rate_limited: %s", recorder.Code, recorder.Body.String())
	}
}

func TestNoPublicRegistrationRoute(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	for _, path := range []string{"/api/v1/auth/register", "/api/v1/users", "/api/v1/register"} {
		recorder := f.do(http.MethodPost, path, map[string]string{"username": "waiting", "password": "some-password-123"}, nil, true)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404 (no public registration)", path, recorder.Code)
		}
	}
}

func TestAdminEndpointsAuthorization(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	f.bootstrapRoot("waiting")
	rootCookie, _ := f.login("waiting", httpRootPassword)

	created := f.do(http.MethodPost, "/api/v1/admin/users", map[string]string{"username": "bob", "password": "account-password-123"}, rootCookie, true)
	if created.Code != http.StatusCreated {
		t.Fatalf("create user status = %d: %s", created.Code, created.Body.String())
	}
	var payload struct {
		Data struct {
			ID         string `json:"id"`
			SystemRoot bool   `json:"system_root"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.SystemRoot {
		t.Fatal("created account must not be ROOT")
	}
	bobID := payload.Data.ID

	bobCookie, _ := f.login("bob", "account-password-123")

	deniedCreate := f.do(http.MethodPost, "/api/v1/admin/users", map[string]string{"username": "mallory", "password": "account-password-123"}, bobCookie, true)
	if deniedCreate.Code != http.StatusForbidden {
		t.Fatalf("ordinary user create = %d, want 403", deniedCreate.Code)
	}
	deniedGrant := f.do(http.MethodPost, "/api/v1/admin/platform-roles", map[string]string{"user_id": bobID}, bobCookie, true)
	if deniedGrant.Code != http.StatusForbidden {
		t.Fatalf("ordinary user grant = %d, want 403", deniedGrant.Code)
	}

	grant := f.do(http.MethodPost, "/api/v1/admin/platform-roles", map[string]string{"user_id": bobID}, rootCookie, true)
	if grant.Code != http.StatusCreated {
		t.Fatalf("root grant = %d: %s", grant.Code, grant.Body.String())
	}
	nowAdmin := f.do(http.MethodPost, "/api/v1/admin/users", map[string]string{"username": "carol", "password": "account-password-123"}, bobCookie, true)
	if nowAdmin.Code != http.StatusCreated {
		t.Fatalf("platform admin create = %d: %s", nowAdmin.Code, nowAdmin.Body.String())
	}
	// Security scenario 16: a team/platform admin cannot grant themselves ROOT.
	selfRoot := f.do(http.MethodPost, "/api/v1/admin/platform-roles", map[string]string{"user_id": bobID}, bobCookie, true)
	if selfRoot.Code != http.StatusForbidden {
		t.Fatalf("platform admin self-grant = %d, want 403", selfRoot.Code)
	}

	revokeRole := f.do(http.MethodDelete, "/api/v1/admin/platform-roles/"+bobID, nil, rootCookie, true)
	if revokeRole.Code != http.StatusOK {
		t.Fatalf("revoke role = %d: %s", revokeRole.Code, revokeRole.Body.String())
	}

	revokeSessions := f.do(http.MethodDelete, "/api/v1/admin/users/"+bobID+"/sessions", nil, rootCookie, true)
	if revokeSessions.Code != http.StatusOK {
		t.Fatalf("revoke sessions = %d: %s", revokeSessions.Code, revokeSessions.Body.String())
	}
	afterRevoke := f.do(http.MethodGet, "/api/v1/auth/me", nil, bobCookie, false)
	if afterRevoke.Code != http.StatusUnauthorized {
		t.Fatalf("bob session survived revocation: %d", afterRevoke.Code)
	}
}

func TestMetaRootInitialized(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	before := f.do(http.MethodGet, "/api/v1/meta", nil, nil, false)
	if before.Code != http.StatusOK || !strings.Contains(before.Body.String(), `"root_initialized":false`) {
		t.Fatalf("meta before bootstrap: %s", before.Body.String())
	}
	f.bootstrapRoot("waiting")
	after := f.do(http.MethodGet, "/api/v1/meta", nil, nil, false)
	if after.Code != http.StatusOK || !strings.Contains(after.Body.String(), `"root_initialized":true`) {
		t.Fatalf("meta after bootstrap: %s", after.Body.String())
	}
}

func TestLoginValidationAndBadBodies(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	missing := f.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "waiting"}, nil, true)
	if missing.Code != http.StatusUnprocessableEntity || errorCode(t, missing) != "validation_failed" {
		t.Fatalf("missing password = %d, want 422", missing.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{not-json"))
	request.Header.Set(auth.CSRFHeader, "1")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", recorder.Code)
	}
}
