package projects

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(s *Service, l *slog.Logger) *Handler { return &Handler{s, l} }
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	b := "/teams/{teamID}/projects"
	r.Get(b, h.list)
	r.Post(b, h.create)
	r.Get(b+"/{projectID}", h.get)
	r.Patch(b+"/{projectID}", h.edit)
	r.Post(b+"/{projectID}/archive", h.archive)
	r.Post(b+"/{projectID}/handoff", h.handoff)
	r.Get(b+"/{projectID}/members", h.members)
	r.Post(b+"/{projectID}/members", h.addMember)
	r.Delete(b+"/{projectID}/members/{userID}", h.removeMember)
	r.Get(b+"/{projectID}/milestones", h.milestones)
	r.Post(b+"/{projectID}/milestones", h.createMilestone)
	r.Patch(b+"/{projectID}/milestones/{milestoneID}", h.editMilestone)
}
func (h *Handler) scope(w http.ResponseWriter, r *http.Request, project bool) (a reqctx.Identity, t, p uuid.UUID, ok bool) {
	a, ok = reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	t, ok = httpx.ParseUUIDParam(w, r, h.logger, "teamID")
	if !ok {
		return
	}
	if project {
		p, ok = httpx.ParseUUIDParam(w, r, h.logger, "projectID")
	}
	return
}
func (h *Handler) reply(w http.ResponseWriter, r *http.Request, data any, err error, status int) {
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, status, data)
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	a, t, _, ok := h.scope(w, r, false)
	if !ok {
		return
	}
	v, e := h.svc.List(r.Context(), a, t)
	h.reply(w, r, v, e, 200)
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	a, t, _, ok := h.scope(w, r, false)
	if !ok {
		return
	}
	var in CreateInput
	if !httpx.DecodeBody(w, r, h.logger, &in) {
		return
	}
	v, e := h.svc.Create(r.Context(), a, t, in)
	h.reply(w, r, v, e, 201)
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	a, t, p, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	v, e := h.svc.Get(r.Context(), a, t, p)
	h.reply(w, r, v, e, 200)
}
func (h *Handler) edit(w http.ResponseWriter, r *http.Request) {
	a, t, p, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	var in EditInput
	if !httpx.DecodeBody(w, r, h.logger, &in) {
		return
	}
	v, e := h.svc.Edit(r.Context(), a, t, p, in)
	h.reply(w, r, v, e, 200)
}
func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	a, t, p, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if !httpx.DecodeBody(w, r, h.logger, &in) {
		return
	}
	v, e := h.svc.Archive(r.Context(), a, t, p, in.ExpectedVersion)
	h.reply(w, r, v, e, 200)
}
func (h *Handler) handoff(w http.ResponseWriter, r *http.Request) {
	a, t, p, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	var in struct {
		LeadID          uuid.UUID `json:"lead_id"`
		Reason          string    `json:"reason"`
		ExpectedVersion int       `json:"expected_version"`
	}
	if !httpx.DecodeBody(w, r, h.logger, &in) {
		return
	}
	v, e := h.svc.Handoff(r.Context(), a, t, p, in.LeadID, in.ExpectedVersion, in.Reason)
	h.reply(w, r, v, e, 200)
}
func (h *Handler) members(w http.ResponseWriter, r *http.Request) {
	a, t, p, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	v, e := h.svc.Members(r.Context(), a, t, p)
	h.reply(w, r, v, e, 200)
}
func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	a, t, p, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	var in struct {
		UserID uuid.UUID `json:"user_id"`
	}
	if !httpx.DecodeBody(w, r, h.logger, &in) {
		return
	}
	v, e := h.svc.ChangeMember(r.Context(), a, t, p, in.UserID, true)
	h.reply(w, r, v, e, 201)
}
func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	a, t, p, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	u, ok := httpx.ParseUUIDParam(w, r, h.logger, "userID")
	if !ok {
		return
	}
	v, e := h.svc.ChangeMember(r.Context(), a, t, p, u, false)
	h.reply(w, r, v, e, 200)
}
func (h *Handler) milestones(w http.ResponseWriter, r *http.Request) {
	a, t, p, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	v, e := h.svc.Milestones(r.Context(), a, t, p)
	h.reply(w, r, v, e, 200)
}
func (h *Handler) createMilestone(w http.ResponseWriter, r *http.Request) {
	h.saveMilestone(w, r, true)
}
func (h *Handler) editMilestone(w http.ResponseWriter, r *http.Request) { h.saveMilestone(w, r, false) }
func (h *Handler) saveMilestone(w http.ResponseWriter, r *http.Request, create bool) {
	a, t, p, ok := h.scope(w, r, true)
	if !ok {
		return
	}
	var in MilestoneInput
	if !httpx.DecodeBody(w, r, h.logger, &in) {
		return
	}
	m := uuid.Nil
	if !create {
		m, ok = httpx.ParseUUIDParam(w, r, h.logger, "milestoneID")
		if !ok {
			return
		}
	}
	v, e := h.svc.SaveMilestone(r.Context(), a, t, p, m, in)
	status := 200
	if create {
		status = 201
	}
	h.reply(w, r, v, e, status)
}
