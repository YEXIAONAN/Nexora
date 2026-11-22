//go:build integration

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

const teamHTTPPassword = httpRootPassword

func (f *httpAPIFixture) rootActor() reqctx.Identity {
	f.t.Helper()
	acct, err := f.svc.AccountByUsername(f.ctx, "root")
	if err != nil {
		f.t.Fatalf("resolve root: %v", err)
	}
	return reqctx.Identity{UserID: acct.ID, Username: acct.Username, SystemRoot: true}
}

func (f *httpAPIFixture) createUser(username string) {
	f.t.Helper()
	if _, err := f.svc.CreateUser(f.ctx, f.rootActor(), username, "", teamHTTPPassword); err != nil {
		f.t.Fatalf("create %s: %v", username, err)
	}
}

func dataMap(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode data: %v (%s)", err, recorder.Body.String())
	}
	return envelope.Data
}

func dataList(t *testing.T, recorder *httptest.ResponseRecorder) []any {
	t.Helper()
	var envelope struct {
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode data list: %v (%s)", err, recorder.Body.String())
	}
	return envelope.Data
}

func (f *httpAPIFixture) currentUserID(cookie *http.Cookie) string {
	f.t.Helper()
	recorder := f.do(http.MethodGet, "/api/v1/auth/me", nil, cookie, false)
	if recorder.Code != http.StatusOK {
		f.t.Fatalf("read current user status = %d: %s", recorder.Code, recorder.Body.String())
	}
	payload := dataMap(f.t, recorder)
	id, ok := payload["id"].(string)
	if !ok {
		f.t.Fatalf("current user payload missing id: %v", payload)
	}
	return id
}

// createTeam provisions a team as the platform administrator and assigns the
// cookie's account as owner. Ordinary accounts can no longer create teams.
func (f *httpAPIFixture) createTeam(ownerCookie *http.Cookie, name string) string {
	f.t.Helper()
	rootCookie, _ := f.login("root", teamHTTPPassword)
	recorder := f.do(http.MethodPost, "/api/v1/teams",
		map[string]any{"name": name, "owner_id": f.currentUserID(ownerCookie)}, rootCookie, true)
	if recorder.Code != http.StatusCreated {
		f.t.Fatalf("create team status = %d: %s", recorder.Code, recorder.Body.String())
	}
	return dataMap(f.t, recorder)["id"].(string)
}

func (f *httpAPIFixture) invite(cookie *http.Cookie, teamID, username, role string) string {
	f.t.Helper()
	recorder := f.do(http.MethodPost, "/api/v1/teams/"+teamID+"/invitations",
		map[string]any{"username": username, "role": role}, cookie, true)
	if recorder.Code != http.StatusCreated {
		f.t.Fatalf("invite status = %d: %s", recorder.Code, recorder.Body.String())
	}
	return dataMap(f.t, recorder)["id"].(string)
}

func (f *httpAPIFixture) accept(cookie *http.Cookie, invitationID string, body any) {
	f.t.Helper()
	recorder := f.do(http.MethodPost, "/api/v1/invitations/"+invitationID+"/accept", body, cookie, true)
	if recorder.Code != http.StatusOK {
		f.t.Fatalf("accept status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func userID(loginData map[string]any) string {
	return loginData["user"].(map[string]any)["id"].(string)
}

func TestTeamRoutesRequireSessionAndCSRF(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	recorder := f.do(http.MethodGet, "/api/v1/teams", nil, nil, false)
	if recorder.Code != http.StatusUnauthorized || errorCode(t, recorder) != "unauthorized" {
		t.Fatalf("teams without session = %d, want 401", recorder.Code)
	}

	f.bootstrapRoot("root")
	cookie, _ := f.login("root", teamHTTPPassword)
	noCSRF := f.do(http.MethodPost, "/api/v1/teams", map[string]string{"name": "No CSRF"}, cookie, false)
	if noCSRF.Code != http.StatusForbidden || errorCode(t, noCSRF) != "forbidden" {
		t.Fatalf("create team without CSRF = %d, want 403", noCSRF.Code)
	}
	f.createTeam(cookie, "With CSRF")
}

func TestCreateTeamRequiresPlatformAdmin(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	f.createUser("alice")
	aliceCookie, _ := f.login("alice", teamHTTPPassword)
	recorder := f.do(http.MethodPost, "/api/v1/teams", map[string]string{"name": "Self service"}, aliceCookie, true)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("ordinary account create team = %d, want 403: %s", recorder.Code, recorder.Body.String())
	}
	rootCookie, _ := f.login("root", teamHTTPPassword)
	provisioned := f.do(http.MethodPost, "/api/v1/teams",
		map[string]any{"name": "Provisioned", "owner_id": f.currentUserID(aliceCookie)}, rootCookie, true)
	if provisioned.Code != http.StatusCreated {
		t.Fatalf("admin provisioned team = %d: %s", provisioned.Code, provisioned.Body.String())
	}
}

// Security scenario 11 (HTTP layer): swapping the team_id in the URL never
// exposes another team's data.
func TestURLTeamIDTamperingIsNotFound(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	f.createUser("alice")
	f.createUser("bob")
	aliceCookie, aliceData := f.login("alice", teamHTTPPassword)
	bobCookie, _ := f.login("bob", teamHTTPPassword)

	teamA := f.createTeam(aliceCookie, "HTTP Team A")
	teamB := f.createTeam(bobCookie, "HTTP Team B")
	invID := f.invite(bobCookie, teamB, "alice", "member")
	f.accept(aliceCookie, invID, nil)

	for _, probe := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/v1/teams/" + teamA, nil},
		{http.MethodGet, "/api/v1/teams/" + teamA + "/members", nil},
		{http.MethodGet, "/api/v1/teams/" + teamA + "/permissions", nil},
		{http.MethodGet, "/api/v1/teams/" + teamA + "/invitations", nil},
		{http.MethodPost, "/api/v1/teams/" + teamA + "/invitations", map[string]any{"username": "bob", "role": "member"}},
		{http.MethodPatch, "/api/v1/teams/" + teamA + "/members/" + userID(aliceData), map[string]string{"role": "member"}},
		{http.MethodDelete, "/api/v1/teams/" + teamA + "/members/" + userID(aliceData), nil},
	} {
		recorder := f.do(probe.method, probe.path, probe.body, bobCookie, true)
		if recorder.Code != http.StatusNotFound || errorCode(t, recorder) != "not_found" {
			t.Errorf("%s %s = %d, want 404 not_found: %s", probe.method, probe.path, recorder.Code, recorder.Body.String())
		}
	}

	// alice's legitimate membership in B still works.
	ok := f.do(http.MethodGet, "/api/v1/teams/"+teamB, nil, aliceCookie, false)
	if ok.Code != http.StatusOK {
		t.Fatalf("alice in B = %d: %s", ok.Code, ok.Body.String())
	}
}

// Security scenario 12 (HTTP layer): a team_id smuggled into the request body
// is rejected, and accepting an invitation only joins the invitation's team.
func TestBodyTeamIDCannotOverridePath(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	f.createUser("alice")
	f.createUser("bob")
	f.createUser("carol")
	aliceCookie, _ := f.login("alice", teamHTTPPassword)
	bobCookie, _ := f.login("bob", teamHTTPPassword)
	carolCookie, _ := f.login("carol", teamHTTPPassword)

	teamA := f.createTeam(aliceCookie, "Body Team A")
	teamB := f.createTeam(bobCookie, "Body Team B")

	smuggled := f.do(http.MethodPost, "/api/v1/teams/"+teamB+"/invitations",
		map[string]any{"username": "carol", "role": "member", "team_id": teamA}, bobCookie, true)
	if smuggled.Code != http.StatusBadRequest || errorCode(t, smuggled) != "bad_request" {
		t.Fatalf("smuggled team_id = %d, want 400: %s", smuggled.Code, smuggled.Body.String())
	}

	invID := f.invite(aliceCookie, teamA, "carol", "member")
	f.accept(carolCookie, invID, map[string]string{"team_id": teamB})

	list := f.do(http.MethodGet, "/api/v1/teams", nil, carolCookie, false)
	if list.Code != http.StatusOK {
		t.Fatalf("carol teams = %d: %s", list.Code, list.Body.String())
	}
	items := dataList(t, list)
	if len(items) != 1 || items[0].(map[string]any)["id"] != teamA {
		t.Fatalf("carol must belong only to team A, got %s", list.Body.String())
	}
	foreign := f.do(http.MethodGet, "/api/v1/teams/"+teamB, nil, carolCookie, false)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("carol in B = %d, want 404", foreign.Code)
	}
}

// Security scenario 19: Phase 02 ships no batch member operations; the routes
// must not exist and nothing may be partially written.
func TestNoBatchMemberRoutes(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	f.createUser("alice")
	f.createUser("bob")
	aliceCookie, _ := f.login("alice", teamHTTPPassword)
	teamA := f.createTeam(aliceCookie, "Batch Team")
	invID := f.invite(aliceCookie, teamA, "bob", "member")
	bobCookie, _ := f.login("bob", teamHTTPPassword)
	f.accept(bobCookie, invID, nil)

	for _, probe := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/teams/" + teamA + "/members/batch"},
		{http.MethodDelete, "/api/v1/teams/" + teamA + "/members"},
		{http.MethodPatch, "/api/v1/teams/" + teamA + "/members"},
	} {
		recorder := f.do(probe.method, probe.path, map[string]any{"user_ids": []string{}}, aliceCookie, true)
		if recorder.Code != http.StatusNotFound && recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want 404/405", probe.method, probe.path, recorder.Code)
		}
	}

	members := f.do(http.MethodGet, "/api/v1/teams/"+teamA+"/members", nil, aliceCookie, false)
	if len(dataList(t, members)) != 2 {
		t.Fatalf("batch probes must not change membership: %s", members.Body.String())
	}
}

func TestAdminTeamsRequiresPlatformRole(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	f.createUser("alice")
	aliceCookie, _ := f.login("alice", teamHTTPPassword)
	rootCookie, _ := f.login("root", teamHTTPPassword)
	f.createTeam(aliceCookie, "Platform Team")

	forbidden := f.do(http.MethodGet, "/api/v1/admin/teams", nil, aliceCookie, false)
	if forbidden.Code != http.StatusForbidden || errorCode(t, forbidden) != "forbidden" {
		t.Fatalf("team owner listing all teams = %d, want 403", forbidden.Code)
	}
	ok := f.do(http.MethodGet, "/api/v1/admin/teams", nil, rootCookie, false)
	if ok.Code != http.StatusOK || len(dataList(t, ok)) != 1 {
		t.Fatalf("root listing all teams = %d: %s", ok.Code, ok.Body.String())
	}
}

// Security scenario 14 (HTTP layer): the same session cookie loses access to
// the removed team immediately and keeps the other team.
func TestSelfLeaveViaHTTP(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	f.createUser("alice")
	f.createUser("bob")
	aliceCookie, aliceData := f.login("alice", teamHTTPPassword)
	bobCookie, _ := f.login("bob", teamHTTPPassword)
	teamA := f.createTeam(aliceCookie, "Leave A")
	teamB := f.createTeam(bobCookie, "Leave B")
	invID := f.invite(bobCookie, teamB, "alice", "member")
	f.accept(aliceCookie, invID, nil)

	leave := f.do(http.MethodDelete, "/api/v1/teams/"+teamB+"/members/"+userID(aliceData), nil, aliceCookie, true)
	if leave.Code != http.StatusOK {
		t.Fatalf("self leave = %d: %s", leave.Code, leave.Body.String())
	}
	gone := f.do(http.MethodGet, "/api/v1/teams/"+teamB, nil, aliceCookie, false)
	if gone.Code != http.StatusNotFound {
		t.Fatalf("removed team access = %d, want 404", gone.Code)
	}
	kept := f.do(http.MethodGet, "/api/v1/teams/"+teamA, nil, aliceCookie, false)
	if kept.Code != http.StatusOK {
		t.Fatalf("other team must stay accessible: %d", kept.Code)
	}
}

func TestRoleChangeViaHTTPRespectsMatrix(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	f.createUser("alice")
	f.createUser("bob")
	aliceCookie, aliceData := f.login("alice", teamHTTPPassword)
	bobCookie, bobData := f.login("bob", teamHTTPPassword)
	teamA := f.createTeam(aliceCookie, "Matrix Team")
	invID := f.invite(aliceCookie, teamA, "bob", "admin")
	f.accept(bobCookie, invID, nil)

	demote := f.do(http.MethodPatch, "/api/v1/teams/"+teamA+"/members/"+userID(bobData),
		map[string]string{"role": "member"}, aliceCookie, true)
	if demote.Code != http.StatusOK {
		t.Fatalf("owner demotes admin = %d: %s", demote.Code, demote.Body.String())
	}

	promote := f.do(http.MethodPatch, "/api/v1/teams/"+teamA+"/members/"+userID(bobData),
		map[string]string{"role": "admin"}, aliceCookie, true)
	if promote.Code != http.StatusOK {
		t.Fatalf("re-promote = %d: %s", promote.Code, promote.Body.String())
	}

	selfGrant := f.do(http.MethodPatch, "/api/v1/teams/"+teamA+"/members/"+userID(bobData),
		map[string]string{"role": "owner"}, bobCookie, true)
	if selfGrant.Code != http.StatusForbidden {
		t.Fatalf("admin self-grants owner = %d, want 403", selfGrant.Code)
	}
	touchOwner := f.do(http.MethodPatch, "/api/v1/teams/"+teamA+"/members/"+userID(aliceData),
		map[string]string{"role": "member"}, bobCookie, true)
	if touchOwner.Code != http.StatusForbidden {
		t.Fatalf("admin changes owner = %d, want 403", touchOwner.Code)
	}
	removeOwner := f.do(http.MethodDelete, "/api/v1/teams/"+teamA+"/members/"+userID(aliceData), nil, bobCookie, true)
	if removeOwner.Code != http.StatusForbidden {
		t.Fatalf("admin removes owner = %d, want 403", removeOwner.Code)
	}

	lastOwner := f.do(http.MethodDelete, "/api/v1/teams/"+teamA+"/members/"+userID(aliceData), nil, aliceCookie, true)
	if lastOwner.Code != http.StatusConflict || errorCode(t, lastOwner) != "conflict" {
		t.Fatalf("last owner self-leave = %d, want 409", lastOwner.Code)
	}
}

func TestInvalidTeamIDParamIsBadRequest(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	cookie, _ := f.login("root", teamHTTPPassword)
	recorder := f.do(http.MethodGet, "/api/v1/teams/not-a-uuid", nil, cookie, false)
	if recorder.Code != http.StatusBadRequest || errorCode(t, recorder) != "bad_request" {
		t.Fatalf("invalid team id = %d, want 400", recorder.Code)
	}
}
