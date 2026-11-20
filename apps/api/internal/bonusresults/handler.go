package bonusresults

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

// AuthenticatedRoutes must be mounted behind the existing session and CSRF
// middleware; the subject is NEVER supplied as a userID path parameter.
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	r.Get("/teams/{teamID}/bonus-results/mine", h.mine)
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	subject, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	teamID, ok := httpx.ParseUUIDParam(w, r, h.logger, "teamID")
	if !ok {
		return
	}
	results, err := h.service.Mine(r.Context(), subject, teamID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, results)
}
