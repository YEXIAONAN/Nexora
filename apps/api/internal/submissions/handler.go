package submissions

import (
	"bytes"
	"errors"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(s *Service, l *slog.Logger) *Handler { return &Handler{s, l} }
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	base := "/teams/{teamID}/projects/{projectID}"
	for _, route := range []struct{ method, path, action string }{
		{"GET", "/tasks/{taskID}/submissions", "versions"}, {"POST", "/tasks/{taskID}/submissions", "submit"},
		{"GET", "/tasks/{taskID}/submissions/{submissionVersionID}", "version"}, {"POST", "/tasks/{taskID}/submissions/{submissionVersionID}/reviews", "review"},
		{"GET", "/tasks/{taskID}/reviews", "reviews"}, {"POST", "/tasks/{taskID}/attachments", "upload"},
		{"DELETE", "/tasks/{taskID}/attachments/{attachmentID}", "discard"}, {"GET", "/tasks/{taskID}/submissions/{submissionVersionID}/attachments/{attachmentID}", "download"},
		{"GET", "/reviewers", "reviewers"}, {"POST", "/reviewers", "grant"}, {"DELETE", "/reviewers/{userID}", "revoke"},
	} {
		r.MethodFunc(route.method, base+route.path, h.handle(route.action))
	}
	r.Get("/teams/{teamID}/reviews/pending", h.handle("pending"))
}
func (h *Handler) handle(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := reqctx.IdentityFrom(r.Context())
		if !ok {
			httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
			return
		}
		ids := map[string]uuid.UUID{}
		for _, key := range []string{"teamID", "projectID", "taskID", "submissionVersionID", "attachmentID", "userID"} {
			if chi.URLParam(r, key) != "" {
				id, ok := httpx.ParseUUIDParam(w, r, h.logger, key)
				if !ok {
					return
				}
				ids[key] = id
			}
		}
		team, pid, task, version := ids["teamID"], ids["projectID"], ids["taskID"], ids["submissionVersionID"]
		var data any
		var err error
		status := http.StatusOK
		switch action {
		case "versions":
			data, err = h.svc.Versions(r.Context(), a, team, pid, task)
		case "version":
			data, err = h.svc.Version(r.Context(), a, team, pid, task, version)
		case "reviews":
			data, err = h.svc.Reviews(r.Context(), a, team, pid, task)
		case "pending":
			data, err = h.svc.Pending(r.Context(), a, team)
		case "reviewers":
			data, err = h.svc.Reviewers(r.Context(), a, team, pid)
		case "submit":
			var in SubmitInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			data, err = h.svc.Submit(r.Context(), a, team, pid, task, in)
			status = 201
		case "review":
			var in ReviewInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			data, err = h.svc.Review(r.Context(), a, team, pid, task, version, in)
			status = 201
		case "grant":
			var in struct {
				UserID uuid.UUID `json:"user_id"`
				Reason string    `json:"reason"`
			}
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			data, err = h.svc.Grant(r.Context(), a, team, pid, in.UserID, in.Reason)
			status = 201
		case "revoke":
			var in struct {
				ExpectedVersion int    `json:"expected_version"`
				Reason          string `json:"reason"`
			}
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			data, err = h.svc.Revoke(r.Context(), a, team, pid, ids["userID"], in.ExpectedVersion, in.Reason)
		case "discard":
			data, err = h.svc.Discard(r.Context(), a, team, pid, task, ids["attachmentID"])
		case "upload":
			key, e := uuid.Parse(r.Header.Get("X-Nexora-Upload-Id"))
			if e != nil {
				err = httpx.BadRequest("X-Nexora-Upload-Id must be a UUID")
				break
			}
			r.Body = http.MaxBytesReader(w, r.Body, MaxFileBytes+(1<<20))
			mr, e := r.MultipartReader()
			if e != nil {
				err = httpx.BadRequest("multipart/form-data with one file is required")
				break
			}
			part, e := mr.NextPart()
			if e != nil {
				err = httpx.BadRequest("one file is required")
				break
			}
			_, params, e := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			if e != nil || params["name"] != "file" || params["filename"] == "" {
				part.Close()
				err = httpx.BadRequest("one file field named file is required")
				break
			}

			contents, e := io.ReadAll(io.LimitReader(part, MaxFileBytes+1))
			part.Close()
			if e != nil {
				err = httpx.BadRequest("invalid upload body")
				break
			}
			if int64(len(contents)) > MaxFileBytes {
				err = httpx.ValidationFailed(map[string]string{"file": "file exceeds 10 MiB"})
				break
			}
			if _, e = mr.NextPart(); !errors.Is(e, io.EOF) {
				err = httpx.BadRequest("exactly one file is required")
				break
			}
			// Validate the original filename, not multipart.FileName's sanitized basename.
			data, err = h.svc.Upload(r.Context(), a, team, pid, task, key, params["filename"], bytes.NewReader(contents))

			status = 201
		case "download":
			meta, file, e := h.svc.Download(r.Context(), a, team, pid, task, version, ids["attachmentID"])
			if e != nil {
				err = e
				break
			}
			defer file.Close()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": meta.FileName}))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "sandbox")
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Content-Length", strconv.FormatInt(meta.ByteSize, 10))
			http.ServeContent(w, r, meta.FileName, meta.CreatedAt, file)
			return
		}
		if err != nil {
			httpx.WriteError(w, r, h.logger, err)
			return
		}
		httpx.Data(w, status, data)
	}
}
