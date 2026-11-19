package observability

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRequestIDGeneratesAndPropagates(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		_, _ = w.Write([]byte(reqctx.RequestID(r.Context())))
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	header := recorder.Header().Get("X-Request-Id")
	if header == "" {
		t.Fatal("response is missing X-Request-Id")
	}
	if got := recorder.Body.String(); got != header {
		t.Fatalf("context id %q does not match response header %q", got, header)
	}
}

func TestRequestIDPreservesValidInboundID(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(reqctx.RequestID(r.Context())))
	}))

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-Id", "client.request-123")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := recorder.Body.String(); got != "client.request-123" {
		t.Fatalf("request id = %q, want the inbound id", got)
	}
}

func TestRequestIDReplacesUnsafeInboundID(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(reqctx.RequestID(r.Context())))
	}))

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-Id", "bad id with spaces \n")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if body == "bad id with spaces \n" || body == "" {
		t.Fatalf("unsafe request id was preserved: %q", body)
	}
}

func TestRecovererReturnsJSON500(t *testing.T) {
	handler := RequestID(Recoverer(testLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if envelope.Error.Code != "internal_error" {
		t.Errorf("code = %q", envelope.Error.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	for _, header := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Strict-Transport-Security"} {
		if recorder.Header().Get(header) == "" {
			t.Errorf("missing security header %s", header)
		}
	}
}

func TestRequestIDDoesNotReflectCookieCredentials(t *testing.T) {
	const credential = "fixture-cookie-credential-not-a-real-session"
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(reqctx.RequestID(r.Context()))) }))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "nexora_session", Value: credential})
	request.Header.Set("X-Request-Id", credential)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("X-Request-Id") == credential || strings.Contains(response.Body.String(), credential) {
		t.Fatal("cookie credential reflected as request correlation data")
	}
}
