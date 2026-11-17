//go:build integration

package httpapi

import (
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"net/http"
	"testing"
)

func TestProjectsTasksHTTPRealWorkflow(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	cookie, me := f.login("root", httpRootPassword)
	team := f.createTeam(cookie, "Phase03")
	base := "/api/v1/teams/" + team + "/projects"
	if r := f.do("GET", base, nil, nil, false); r.Code != 401 {
		t.Fatalf("missing auth %d", r.Code)
	}
	if r := f.do("POST", base, map[string]any{"name": "x", "lead_id": userID(me)}, cookie, false); r.Code != 403 {
		t.Fatalf("missing CSRF %d", r.Code)
	}
	if r := f.do("POST", base, map[string]any{"name": "x", "lead_id": userID(me), "team_id": team}, cookie, true); r.Code != 400 {
		t.Fatalf("body team override %d", r.Code)
	}
	r := f.do("POST", base, map[string]any{"name": "Real project", "lead_id": userID(me)}, cookie, true)
	if r.Code != 201 {
		t.Fatalf("project %d %s", r.Code, r.Body.String())
	}
	project := dataMap(t, r)
	p := base + "/" + project["id"].(string)
	r = f.do("POST", p+"/tasks", map[string]any{"title": "Task", "owner_id": userID(me), "weight": 3}, cookie, true)
	if r.Code != 201 {
		t.Fatalf("task %d %s", r.Code, r.Body.String())
	}
	task := dataMap(t, r)
	taskPath := p + "/tasks/" + task["id"].(string)
	for _, path := range []string{p, p + "/members", p + "/milestones", p + "/tasks", p + "/progress", p + "/progress/history", taskPath, taskPath + "/history"} {
		r = f.do("GET", path, nil, cookie, false)
		if r.Code != 200 {
			t.Fatalf("GET %s %d %s", path, r.Code, r.Body.String())
		}
	}
	r = f.do("POST", taskPath+"/transition", map[string]any{"status": "completed", "expected_version": 1}, cookie, true)
	if r.Code != 409 || errorCode(t, r) != "conflict" {
		t.Fatalf("completion bypass %d", r.Code)
	}
	r = f.do("POST", taskPath+"/transition", map[string]any{"status": "in_progress", "expected_version": 1}, cookie, true)
	if r.Code != 200 {
		t.Fatalf("start %d", r.Code)
	}
	r = f.do("PUT", taskPath+"/progress", map[string]any{"actual_progress": 99, "expected_version": 2}, cookie, true)
	if r.Code != 200 {
		t.Fatalf("progress %d %s", r.Code, r.Body.String())
	}
	r = f.do("GET", p+"/progress", nil, cookie, false)
	if dataMap(t, r)["progress_bps"] != float64(7900) {
		t.Fatal("wrong authority progress")
	}
	r = f.do("PUT", taskPath+"/weight", map[string]any{"weight": 9, "reason": "approved estimate", "expected_version": 3}, cookie, true)
	if r.Code != 200 {
		t.Fatalf("weight %d %s", r.Code, r.Body.String())
	}
	r = f.do("POST", p+"/archive", map[string]any{"expected_version": 1}, cookie, true)
	if r.Code != 200 {
		t.Fatalf("archive %d", r.Code)
	}
	for _, probe := range []struct {
		method, path string
		body         any
	}{
		{"POST", p + "/tasks", map[string]any{"title": "late", "owner_id": userID(me), "weight": 1}},
		{"PUT", taskPath + "/progress", map[string]any{"actual_progress": 10, "expected_version": 4}},
		{"POST", taskPath + "/transition", map[string]any{"status": "cancelled", "reason": "late", "expected_version": 4}},
		{"POST", p + "/members", map[string]any{"user_id": userID(me)}},
		{"POST", p + "/milestones", map[string]any{"name": "late"}},
	} {
		r = f.do(probe.method, probe.path, probe.body, cookie, true)
		if r.Code != http.StatusConflict {
			t.Fatalf("archived write %s %d %s", probe.path, r.Code, r.Body.String())
		}
	}
}
