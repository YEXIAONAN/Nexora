package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
)

type fakeSystemStore struct {
	pingErr        error
	schemaVersion  int64
	schemaErr      error
	installationID string
	installErr     error
}

func (f *fakeSystemStore) Ping(context.Context) error { return f.pingErr }
func (f *fakeSystemStore) SchemaVersion(context.Context) (int64, error) {
	return f.schemaVersion, f.schemaErr
}
func (f *fakeSystemStore) InstallationID(context.Context) (string, error) {
	return f.installationID, f.installErr
}

func newTestServer(store SystemStore) *Server {
	return New(config.Config{Environment: config.EnvironmentTest}, slog.New(slog.NewTextHandler(io.Discard, nil)), store)
}

func performRequest(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

func TestLiveness(t *testing.T) {
	recorder := performRequest(t, newTestServer(&fakeSystemStore{}).Handler(), http.MethodGet, "/healthz")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if recorder.Header().Get("X-Request-Id") == "" {
		t.Error("missing request id header")
	}
}

func TestReadinessReady(t *testing.T) {
	recorder := performRequest(t, newTestServer(&fakeSystemStore{schemaVersion: 1}).Handler(), http.MethodGet, "/readyz")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var payload readinessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Status != "ready" {
		t.Errorf("status = %q", payload.Status)
	}
	if payload.Checks["database"].Status != "up" || payload.Checks["schema"].Status != "up" {
		t.Errorf("unexpected checks: %+v", payload.Checks)
	}
}

func TestReadinessUnavailableWhenDatabaseDown(t *testing.T) {
	store := &fakeSystemStore{schemaVersion: 1, pingErr: errors.New("connection refused")}
	recorder := performRequest(t, newTestServer(store).Handler(), http.MethodGet, "/readyz")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	var payload readinessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Checks["database"].Status != "down" {
		t.Errorf("database check = %+v", payload.Checks["database"])
	}
	if strings.Contains(recorder.Body.String(), "connection refused") {
		t.Errorf("internal error leaked: %s", recorder.Body.String())
	}
}

func TestReadinessUnavailableWhenSchemaMissing(t *testing.T) {
	store := &fakeSystemStore{schemaErr: errors.New("relation goose_db_version does not exist")}
	recorder := performRequest(t, newTestServer(store).Handler(), http.MethodGet, "/readyz")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestMeta(t *testing.T) {
	store := &fakeSystemStore{installationID: "7c1f3f5e-0000-4000-8000-000000000000"}
	recorder := performRequest(t, newTestServer(store).Handler(), http.MethodGet, "/api/v1/meta")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var payload struct {
		Data metaData `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Data.Name != "Nexora" {
		t.Errorf("name = %q", payload.Data.Name)
	}
	if payload.Data.InstanceID == nil || *payload.Data.InstanceID != store.installationID {
		t.Errorf("instance id = %v", payload.Data.InstanceID)
	}
	if payload.Data.Environment != "test" {
		t.Errorf("environment = %q", payload.Data.Environment)
	}
}

func TestMetaToleratesMissingInstallationID(t *testing.T) {
	store := &fakeSystemStore{installErr: errors.New("not initialized")}
	recorder := performRequest(t, newTestServer(store).Handler(), http.MethodGet, "/api/v1/meta")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var payload struct {
		Data metaData `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Data.InstanceID != nil {
		t.Errorf("instance id should be null, got %v", *payload.Data.InstanceID)
	}
}

func TestUnknownRouteReturnsJSON(t *testing.T) {
	recorder := performRequest(t, newTestServer(&fakeSystemStore{}).Handler(), http.MethodGet, "/api/v1/does-not-exist")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if envelope.Error.Code != "not_found" {
		t.Errorf("code = %q", envelope.Error.Code)
	}
}
