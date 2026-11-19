package bonusrounds

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

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

// AuthenticatedRoutes mounts read-only round metadata behind the existing
// session middleware. There is no command route in this phase.
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	base := "/teams/{teamID}/projects/{projectID}/bonus-rounds"
	r.Get(base, h.list)
	r.Get(base+"/{roundID}", h.get)
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
