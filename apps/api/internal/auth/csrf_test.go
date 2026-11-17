package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFBlocksMutationsWithoutHeader(t *testing.T) {
	handler := CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, "/api/v1/teams", nil))
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s without header: status = %d, want 403", method, recorder.Code)
		}
	}
}

func TestCSRFAllowsMutationsWithHeader(t *testing.T) {
	handler := CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(method, "/api/v1/teams", nil)
		request.Header.Set(CSRFHeader, "1")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s with header: status = %d, want 200", method, recorder.Code)
		}
	}
}

func TestCSRFPassesSafeMethods(t *testing.T) {
	handler := CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, "/api/v1/meta", nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", method, recorder.Code)
		}
	}
}

func TestCSRFRejectsWrongHeaderValue(t *testing.T) {
	handler := CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/teams", nil)
	request.Header.Set(CSRFHeader, "true")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}
