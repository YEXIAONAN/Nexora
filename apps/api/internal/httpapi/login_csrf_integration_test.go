//go:build integration

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
)

// rawLogin posts an arbitrary body with an explicit Content-Type and CSRF
// choice through the real router, reproducing what a browser or a cross-site
// form actually sends rather than calling the middleware in isolation.
func (f *httpAPIFixture) rawLogin(body, contentType string, csrf bool) *httptest.ResponseRecorder {
	f.t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if csrf {
		request.Header.Set(auth.CSRFHeader, "1")
	}
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func sessionCookieOf(recorder *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName {
			return cookie
		}
	}
	return nil
}

// A login POST without the custom header is rejected even with valid JSON
// credentials, and never establishes a session.
func TestLoginRequiresCSRFHeader(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	f.bootstrapRoot("waiting")
	recorder := f.rawLogin(`{"username":"waiting","password":"`+httpRootPassword+`"}`, "application/json", false)
	if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "forbidden" {
		t.Fatalf("login without CSRF = %d, want 403: %s", recorder.Code, recorder.Body.String())
	}
	if sessionCookieOf(recorder) != nil {
		t.Fatal("rejected login must not set a session cookie")
	}
}

// The first login carries no session cookie yet may legally send the custom
// header; the success cookie keeps its HttpOnly/SameSite/host-only boundary.
func TestLoginWithCSRFHeaderSucceeds(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	f.bootstrapRoot("waiting")
	recorder := f.rawLogin(`{"username":"waiting","password":"`+httpRootPassword+`"}`, "application/json", true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("first login with CSRF = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	cookie := sessionCookieOf(recorder)
	if cookie == nil {
		t.Fatal("successful login must set the session cookie")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" || cookie.Path != "/" {
		t.Fatalf("login cookie security boundary changed: %+v", cookie)
	}
}

// A non-JSON Content-Type is refused even when the CSRF header is present, so
// the Content-Type guard is an independent layer behind the header check.
func TestLoginRejectsNonJSONContentType(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	f.bootstrapRoot("waiting")
	body := `{"username":"waiting","password":"` + httpRootPassword + `"}`
	for _, contentType := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data", ""} {
		recorder := f.rawLogin(body, contentType, true)
		if recorder.Code != http.StatusBadRequest || errorCode(t, recorder) != "bad_request" {
			t.Errorf("Content-Type %q = %d, want 400: %s", contentType, recorder.Code, recorder.Body.String())
		}
		if sessionCookieOf(recorder) != nil {
			t.Errorf("Content-Type %q must not establish a session", contentType)
		}
	}
}

// Reproduces the reviewed attack: a cross-site top-level form with
// enctype="text/plain" whose name=value separator lands inside a password the
// attacker chose to contain "=", yielding a valid JSON credential pair. The
// form cannot set the custom header, so login is blocked; the same body over
// proper JSON still succeeds, proving only the transport guards stop the form.
func TestLoginCrossSiteFormSpoofIsBlocked(t *testing.T) {
	f := newAuthAPIFixture(t, config.EnvironmentTest, 10)
	const spoofPassword = "spoof=password-1"
	if _, err := f.svc.BootstrapRoot(f.ctx, "waiting", spoofPassword); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	spoofedBody := `{"username":"waiting","password":"spoof=password-1"}`

	form := f.rawLogin(spoofedBody, "text/plain", false)
	if form.Code != http.StatusForbidden || errorCode(t, form) != "forbidden" {
		t.Fatalf("cross-site form login = %d, want 403: %s", form.Code, form.Body.String())
	}
	if sessionCookieOf(form) != nil {
		t.Fatal("cross-site form must not establish a session")
	}

	withHeader := f.rawLogin(spoofedBody, "text/plain", true)
	if withHeader.Code != http.StatusBadRequest {
		t.Fatalf("text/plain with header = %d, want 400: %s", withHeader.Code, withHeader.Body.String())
	}
	if sessionCookieOf(withHeader) != nil {
		t.Fatal("text/plain body must not establish a session")
	}

	legit := f.rawLogin(spoofedBody, "application/json", true)
	if legit.Code != http.StatusOK || sessionCookieOf(legit) == nil {
		t.Fatalf("legit JSON login = %d, want 200 + cookie: %s", legit.Code, legit.Body.String())
	}
}
