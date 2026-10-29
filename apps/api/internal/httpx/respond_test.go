package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

func decodeEnvelope(t *testing.T, body []byte) errorEnvelope {
	t.Helper()
	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope
}

func TestWriteErrorMapsKnownErrors(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	request = request.WithContext(reqctx.WithRequestID(request.Context(), "req-123"))

	WriteError(recorder, request, nil, ValidationFailed(map[string]string{"name": "required"}))

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
	envelope := decodeEnvelope(t, recorder.Body.Bytes())
	if envelope.Error.Code != "validation_failed" {
		t.Errorf("code = %q", envelope.Error.Code)
	}
	if envelope.Error.Fields["name"] != "required" {
		t.Errorf("fields not preserved: %+v", envelope.Error.Fields)
	}
	if envelope.Error.RequestID != "req-123" {
		t.Errorf("request id = %q", envelope.Error.RequestID)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("cache control = %q", got)
	}
}

func TestWriteErrorHidesInternalCause(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)

	WriteError(recorder, request, nil, Internal(errors.New("password authentication failed for user nexora")))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "password authentication failed") {
		t.Fatalf("internal details leaked to client: %s", body)
	}
	envelope := decodeEnvelope(t, recorder.Body.Bytes())
	if envelope.Error.Code != "internal_error" {
		t.Errorf("code = %q", envelope.Error.Code)
	}
}

func TestDataUsesEnvelope(t *testing.T) {
	recorder := httptest.NewRecorder()
	Data(recorder, http.StatusOK, map[string]string{"name": "Nexora"})

	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("content type = %q", got)
	}
	var payload struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Data["name"] != "Nexora" {
		t.Errorf("unexpected payload: %+v", payload)
	}
}
