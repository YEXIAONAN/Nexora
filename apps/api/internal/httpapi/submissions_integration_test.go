//go:build integration

package httpapi

import (
	"bytes"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/google/uuid"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubmissionHTTPAndPrivateDownloadHeaders(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 100)
	f.bootstrapRoot("root")
	cookie, me := f.login("root", httpRootPassword)
	team := f.createTeam(cookie, "Phase04 HTTP")
	base := "/api/v1/teams/" + team + "/projects"
	r := f.do("POST", base, map[string]any{"name": "Evidence", "lead_id": userID(me)}, cookie, true)
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	p := base + "/" + dataMap(t, r)["id"].(string)
	r = f.do("POST", p+"/tasks", map[string]any{"title": "Delivery", "owner_id": userID(me), "weight": 1}, cookie, true)
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	task := p + "/tasks/" + dataMap(t, r)["id"].(string)
	r = f.do("POST", task+"/transition", map[string]any{"status": "in_progress", "expected_version": 1}, cookie, true)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	upload := func(name string, csrf bool) *httptest.ResponseRecorder {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, e := mw.CreateFormFile("file", name)
		if e != nil {
			t.Fatal(e)
		}
		part.Write([]byte("# Actual evidence\nSafe private download\n"))
		mw.Close()
		req := httptest.NewRequest("POST", task+"/attachments", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("X-Nexora-Upload-Id", uuid.NewString())
		req.AddCookie(cookie)
		if csrf {
			req.Header.Set(auth.CSRFHeader, "1")
		}
		out := httptest.NewRecorder()
		f.handler.ServeHTTP(out, req)
		return out
	}
	if r = upload("evidence.md", false); r.Code != 403 {
		t.Fatal("upload missing CSRF", r.Code)
	}
	if r = upload("../escape.txt", true); r.Code != 422 {
		t.Fatal("raw filename traversal", r.Code, r.Body.String())
	}
	r = upload("成果.md", true)
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	attachment := dataMap(t, r)["id"].(string)
	r = f.do("POST", task+"/submissions", map[string]any{"client_request_id": uuid.NewString(), "expected_version": 2, "summary": "Delivered", "attachment_ids": []string{attachment}}, cookie, true)
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	v := dataMap(t, r)
	versionPath := task + "/submissions/" + v["id"].(string)
	for _, path := range []string{task + "/submissions", versionPath, task + "/reviews", p + "/reviewers", "/api/v1/teams/" + team + "/reviews/pending"} {
		if r = f.do("GET", path, nil, cookie, false); r.Code != 200 {
			t.Fatal(path, r.Code, r.Body.String())
		}
	}
	download := versionPath + "/attachments/" + attachment
	r = f.do("GET", download, nil, cookie, false)
	if r.Code != 200 || !strings.Contains(r.Body.String(), "Actual evidence") || r.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(r.Header().Get("Content-Disposition"), "attachment;") || r.Header().Get("X-Content-Type-Options") != "nosniff" || r.Header().Get("Cache-Control") != "private, no-store" || r.Header().Get("Content-Security-Policy") != "sandbox" {
		t.Fatal(r.Code, r.Header(), r.Body.String())
	}
	if r = f.do("GET", download, nil, nil, false); r.Code != 401 {
		t.Fatal("anonymous download", r.Code)
	}
	r = f.do("POST", versionPath+"/reviews", map[string]any{"expected_version": 3, "decision": "approve", "comment": "Self review"}, cookie, true)
	if r.Code != 403 {
		t.Fatal("self review", r.Code, r.Body.String())
	}
	r = f.do("POST", task+"/transition", map[string]any{"expected_version": 3, "status": "completed"}, cookie, true)
	if r.Code != 409 {
		t.Fatal("completion bypass", r.Code)
	}
	if r = f.do(http.MethodDelete, task+"/attachments/"+attachment, nil, cookie, true); r.Code != 409 {
		t.Fatal("bound delete", r.Code)
	}
}
