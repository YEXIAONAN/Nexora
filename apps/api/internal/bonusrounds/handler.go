package bonusrounds

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// AuthenticatedRoutes uses the existing session and CSRF middleware. Metadata
// remains member-readable; the database separately gates every amount view.
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	base := "/teams/{teamID}/projects/{projectID}/bonus-rounds"
	r.Get("/teams/{teamID}/projects/{projectID}/bonus-workspace", h.projectWorkspace)
	r.Get(base, h.list)
	r.Post(base, h.command("create"))
	r.Get(base+"/{roundID}", h.get)
	r.Get(base+"/{roundID}/workspace", h.workspace)
	r.Put(base+"/{roundID}/plan", h.command("plan"))
	for _, action := range []string{"submit", "return", "confirm", "publish", "revise", "payments", "grants"} {
		r.Post(base+"/{roundID}/"+action, h.command(action))
	}
}

func (h *Handler) workflowScope(w http.ResponseWriter, r *http.Request, round bool) (reqctx.Identity, uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return identity, uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	team, ok := httpx.ParseUUIDParam(w, r, h.logger, "teamID")
	if !ok {
		return identity, uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	project, ok := httpx.ParseUUIDParam(w, r, h.logger, "projectID")
	if !ok {
		return identity, team, uuid.Nil, uuid.Nil, false
	}
	id := uuid.Nil
	if round {
		id, ok = httpx.ParseUUIDParam(w, r, h.logger, "roundID")
	}
	return identity, team, project, id, ok
}

func (h *Handler) projectWorkspace(w http.ResponseWriter, r *http.Request) {
	identity, team, project, _, ok := h.workflowScope(w, r, false)
	if !ok {
		return
	}
	out, err := h.service.ProjectWorkspace(r.Context(), identity, team, project)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, out)
}
func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) {
	identity, team, project, round, ok := h.workflowScope(w, r, true)
	if !ok {
		return
	}
	out, err := h.service.Workspace(r.Context(), identity, team, project, round)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, out)
}
func (h *Handler) command(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, team, project, round, ok := h.workflowScope(w, r, action != "create")
		if !ok {
			return
		}
		var input any
		switch action {
		case "create":
			var in CreateInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			input = in
		case "plan":
			var in PlanInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			input = in
		case "payments":
			var in PaymentInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			input = in
		case "grants":
			var in GrantInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			input = in
		default:
			var in Command
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			input = in
		}
		out, err := h.service.Execute(r.Context(), identity, team, project, round, action, input)
		if err != nil {
			httpx.WriteError(w, r, h.logger, err)
			return
		}
		code := http.StatusOK
		if action == "create" {
			code = http.StatusCreated
		}
		httpx.Data(w, code, out)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	teamID, ok := httpx.ParseUUIDParam(w, r, h.logger, "teamID")
	if !ok {
		return
	}
	projectID, ok := httpx.ParseUUIDParam(w, r, h.logger, "projectID")
	if !ok {
		return
	}
	out, err := h.service.List(r.Context(), identity, teamID, projectID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	teamID, ok := httpx.ParseUUIDParam(w, r, h.logger, "teamID")
	if !ok {
		return
	}
	projectID, ok := httpx.ParseUUIDParam(w, r, h.logger, "projectID")
	if !ok {
		return
	}
	roundID, ok := httpx.ParseUUIDParam(w, r, h.logger, "roundID")
	if !ok {
		return
	}
	out, err := h.service.Get(r.Context(), identity, teamID, projectID, roundID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, out)
}
