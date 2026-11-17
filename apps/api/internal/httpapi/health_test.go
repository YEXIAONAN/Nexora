package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

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
	return New(config.Config{Environment: config.EnvironmentTest}, slog.New(slog.NewTextHandler(io.Discard, nil)), store, nil, nil, nil)
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

func TestDiagnosticLogsDoNotExposeDatabaseErrorDetails(t *testing.T) {
	const marker = "SYNTHETIC_DIAGNOSTIC_SECRET"
	var logs bytes.Buffer
	store := &fakeSystemStore{
		pingErr: errors.New(marker), schemaErr: errors.New(marker), installErr: errors.New(marker),
	}
	server := New(config.Config{Environment: config.EnvironmentTest}, slog.New(slog.NewJSONHandler(&logs, nil)), store, nil, nil, nil)
	for _, path := range []string{"/readyz", "/api/v1/meta"} {
		response := performRequest(t, server.Handler(), http.MethodGet, path)
		if strings.Contains(response.Body.String(), marker) {
			t.Fatal("diagnostic response exposed synthetic credentials")
		}
	}
	if strings.Contains(logs.String(), marker) {
		t.Fatal("diagnostic log exposed synthetic database error details")
	}
	if !strings.Contains(logs.String(), "database operation failed") {
		t.Fatal("diagnostic log must retain a safe error category")
	}
}

func TestDiagnosticResponseContracts(t *testing.T) {
	for _, tc := range []struct {
		name, path, state string
		store             fakeSystemStore
		status            int
		keys              []string
	}{
		{"liveness", "/healthz", "ok", fakeSystemStore{}, 200, []string{"checked_at", "status"}},
		{"ready", "/readyz", "ready", fakeSystemStore{schemaVersion: 1}, 200, []string{"checked_at", "checks", "status"}},
		{"not migrated", "/readyz", "unavailable", fakeSystemStore{}, 503, []string{"checked_at", "checks", "status"}},
		{"metadata", "/api/v1/meta", "", fakeSystemStore{installErr: errors.New("not initialized")}, 200, []string{"data"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := performRequest(t, newTestServer(&tc.store).Handler(), http.MethodGet, tc.path)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if response.Header().Get("Content-Type") != "application/json; charset=utf-8" || response.Header().Get("X-Request-Id") == "" {
				t.Fatal("diagnostic response headers changed")
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			keys := make([]string, 0, len(body))
			for key := range body {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			if strings.Join(keys, ",") != strings.Join(tc.keys, ",") {
				t.Fatal("diagnostic response envelope changed")
			}
			if tc.path == "/api/v1/meta" {
				var meta map[string]json.RawMessage
				if err := json.Unmarshal(body["data"], &meta); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"name", "version", "commit", "build_time", "environment", "instance_id", "server_time"} {
					if _, ok := meta[key]; !ok {
						t.Fatalf("metadata required field missing: %s", key)
					}
				}
				if string(meta["instance_id"]) != "null" {
					t.Fatal("missing instance ID must remain explicit null")
				}
				return
			}
			var state, checkedAt string
			if json.Unmarshal(body["status"], &state) != nil || state != tc.state {
				t.Fatal("diagnostic status contract changed")
			}
			if json.Unmarshal(body["checked_at"], &checkedAt) != nil {
				t.Fatal("checked_at must be a string")
			}
			if _, err := time.Parse(time.RFC3339Nano, checkedAt); err != nil {
				t.Fatal("checked_at must be RFC3339")
			}
			if tc.path == "/readyz" {
				var checks map[string]checkResult
				if json.Unmarshal(body["checks"], &checks) != nil || len(checks) != 2 {
					t.Fatal("readiness checks contract changed")
				}
				for _, key := range []string{"database", "schema"} {
					if checks[key].Status != "up" && checks[key].Status != "down" {
						t.Fatal("invalid readiness check status")
					}
				}
			}
		})
	}
}
