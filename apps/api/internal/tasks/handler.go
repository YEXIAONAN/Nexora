package tasks

import (
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(s *Service, l *slog.Logger) *Handler { return &Handler{s, l} }
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	b := "/teams/{teamID}/projects/{projectID}"
	for _, route := range []struct{ method, path, action string }{
		{"GET", "/tasks", "tree"}, {"POST", "/tasks", "create"}, {"GET", "/tasks/{taskID}", "get"}, {"PUT", "/tasks/{taskID}", "edit"},
		{"POST", "/tasks/{taskID}/move", "move"}, {"POST", "/tasks/{taskID}/transition", "transition"}, {"PUT", "/tasks/{taskID}/progress", "progress"},
		{"GET", "/tasks/{taskID}/history", "history"}, {"GET", "/progress", "project_progress"}, {"GET", "/progress/history", "snapshots"},
	} {
		r.MethodFunc(route.method, b+route.path, h.handle(route.action))
	}
}
func (h *Handler) handle(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := reqctx.IdentityFrom(r.Context())
		if !ok {
			httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
			return
		}
		team, ok := httpx.ParseUUIDParam(w, r, h.logger, "teamID")
		if !ok {
			return
		}
		pid, ok := httpx.ParseUUIDParam(w, r, h.logger, "projectID")
		if !ok {
			return
		}
		id := uuid.Nil
		if chi.URLParam(r, "taskID") != "" {
			id, ok = httpx.ParseUUIDParam(w, r, h.logger, "taskID")
			if !ok {
				return
			}
		}
		var data any
		var err error
		status := 200
		switch action {
		case "tree":
			data, err = h.svc.Tree(r.Context(), a, team, pid)
		case "get":
			data, err = h.svc.Get(r.Context(), a, team, pid, id)
		case "create":
			var in CreateInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			data, err = h.svc.Create(r.Context(), a, team, pid, in)
			status = 201
		case "edit":
			var in EditInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			data, err = h.svc.Edit(r.Context(), a, team, pid, id, in)
		case "move":
			var in struct {
				ParentID        *uuid.UUID `json:"parent_id"`
				ExpectedVersion int        `json:"expected_version"`
			}
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			data, err = h.svc.Move(r.Context(), a, team, pid, id, in.ParentID, in.ExpectedVersion)
		case "transition":
			var in struct {
				Status          string `json:"status"`
				Reason          string `json:"reason"`
				ExpectedVersion int    `json:"expected_version"`
			}
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			data, err = h.svc.Transition(r.Context(), a, team, pid, id, in.Status, in.Reason, in.ExpectedVersion)
		case "progress":
			var in struct {
				ActualProgress  *int `json:"actual_progress"`
				ExpectedVersion int  `json:"expected_version"`
			}
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			if in.ActualProgress == nil {
				err = httpx.ValidationFailed(map[string]string{"actual_progress": "is required"})
			} else {
				data, err = h.svc.Progress(r.Context(), a, team, pid, id, *in.ActualProgress, in.ExpectedVersion)
			}
		case "history":
			data, err = h.svc.History(r.Context(), a, team, pid, id)
		case "project_progress":
			data, err = h.svc.ProjectProgress(r.Context(), a, team, pid)
		case "snapshots":
			data, err = h.svc.Snapshots(r.Context(), a, team, pid)
		}
		if err != nil {
			httpx.WriteError(w, r, h.logger, err)
			return
		}
		httpx.Data(w, status, data)
	}
}
