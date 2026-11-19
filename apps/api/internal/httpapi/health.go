package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/version"
)

const readinessTimeout = 3 * time.Second

type livenessResponse struct {
	Status    string    `json:"status"`
	CheckedAt time.Time `json:"checked_at"`
}

type checkResult struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type readinessResponse struct {
	Status    string                 `json:"status"`
	Checks    map[string]checkResult `json:"checks"`
	CheckedAt time.Time              `json:"checked_at"`
}

func (s *Server) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, livenessResponse{Status: "ok", CheckedAt: time.Now().UTC()})
}

// handleReadiness reports whether the instance can serve traffic: the database
// must be reachable and the schema must be migrated. It deliberately returns a
// generic detail while the real error is logged.
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	checks := make(map[string]checkResult, 2)
	ready := true

	if err := s.system.Ping(ctx); err != nil {
		ready = false
		checks["database"] = checkResult{Status: "down", Detail: "connection failed"}
		s.logger.Warn("readiness: database ping failed", "error", database.SafeError(err))
	} else {
		checks["database"] = checkResult{Status: "up"}
	}

	schemaVersion, err := s.system.SchemaVersion(ctx)
	switch {
	case err != nil:
		ready = false
		checks["schema"] = checkResult{Status: "down", Detail: "not migrated"}
		s.logger.Warn("readiness: schema check failed", "error", database.SafeError(err))
	case schemaVersion == 0:
		ready = false
		checks["schema"] = checkResult{Status: "down", Detail: "not migrated"}
	case !database.CompatibleSchemaVersion(schemaVersion):
		ready = false
		checks["schema"] = checkResult{Status: "down", Detail: fmt.Sprintf("version %d (requires v%d–v%d)", schemaVersion, database.MinimumSupportedSchemaVersion, database.MaximumSupportedSchemaVersion)}
	default:
		checks["schema"] = checkResult{Status: "up", Detail: fmt.Sprintf("version %d", schemaVersion)}
	}

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "unavailable"
	}
	httpx.JSON(w, status, readinessResponse{Status: state, Checks: checks, CheckedAt: time.Now().UTC()})
}

type metaData struct {
	Name            string    `json:"name"`
	Version         string    `json:"version"`
	Commit          string    `json:"commit"`
	BuildTime       string    `json:"build_time"`
	Environment     string    `json:"environment"`
	InstanceID      *string   `json:"instance_id"`
	RootInitialized *bool     `json:"root_initialized"`
	ServerTime      time.Time `json:"server_time"`
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	build := version.Get()
	payload := metaData{
		Name:        "Nexora",
		Version:     build.Version,
		Commit:      build.Commit,
		BuildTime:   build.BuildTime,
		Environment: string(s.cfg.Environment),
		ServerTime:  time.Now().UTC(),
	}

	installationID, err := s.system.InstallationID(ctx)
	switch {
	case err == nil:
		payload.InstanceID = &installationID
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		s.logger.Warn("meta: installation id lookup timed out", "error", database.SafeError(err))
	default:
		s.logger.Warn("meta: installation id unavailable", "error", database.SafeError(err))
	}

	if s.authSvc != nil {
		initialized, err := s.authSvc.RootInitialized(ctx)
		if err == nil {
			payload.RootInitialized = &initialized
		} else {
			s.logger.Warn("meta: root initialization state unavailable", "error", database.SafeError(err))
		}
	}

	httpx.Data(w, http.StatusOK, payload)
}
