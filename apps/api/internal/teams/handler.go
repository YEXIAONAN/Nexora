package teams

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

// Handler maps HTTP requests to the teams service. All authorization happens
// in the service; handlers only parse transport input and shape responses.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// AuthenticatedRoutes registers team endpoints. They must be mounted behind
// the session middleware and CSRF protection.
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	r.Get("/teams", h.HandleListMyTeams)
	r.Post("/teams", h.HandleCreateTeam)
	r.Get("/teams/{teamID}", h.HandleGetTeam)
	r.Patch("/teams/{teamID}", h.HandleUpdateTeam)
	r.Get("/teams/{teamID}/permissions", h.HandlePermissions)
	r.Get("/teams/{teamID}/members", h.HandleListMembers)
	r.Post("/teams/{teamID}/invitations", h.HandleInvite)
	r.Get("/teams/{teamID}/invitations", h.HandleListTeamInvitations)
	r.Delete("/teams/{teamID}/invitations/{invitationID}", h.HandleCancelInvitation)
	r.Patch("/teams/{teamID}/members/{userID}", h.HandleChangeMemberRole)
	r.Delete("/teams/{teamID}/members/{userID}", h.HandleRemoveMember)
	r.Get("/invitations", h.HandleListMyInvitations)
	r.Post("/invitations/{invitationID}/accept", h.HandleAcceptInvitation)
	r.Post("/invitations/{invitationID}/decline", h.HandleDeclineInvitation)
	r.Get("/admin/teams", h.HandleListAllTeams)
}

type teamJSON struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedBy   uuid.UUID `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	MyRole      string    `json:"my_role,omitempty"`
}

func toTeamJSON(t Team) teamJSON {
	return teamJSON{ID: t.ID, Name: t.Name, Description: t.Description, CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

type memberJSON struct {
	UserID      uuid.UUID `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
}

type invitationJSON struct {
	ID              uuid.UUID `json:"id"`
	TeamID          uuid.UUID `json:"team_id"`
	TeamName        string    `json:"team_name,omitempty"`
	InvitedUserID   uuid.UUID `json:"invited_user_id"`
	InvitedUsername string    `json:"invited_username"`
	InvitedBy       uuid.UUID `json:"invited_by"`
	Role            string    `json:"role"`
	Status          string    `json:"status"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
}

func toInvitationJSON(inv Invitation) invitationJSON {
	return invitationJSON{
		ID: inv.ID, TeamID: inv.TeamID, TeamName: inv.TeamName,
		InvitedUserID: inv.InvitedUserID, InvitedUsername: inv.InvitedUsername, InvitedBy: inv.InvitedBy,
		Role: inv.Role, Status: inv.Status, ExpiresAt: inv.ExpiresAt, CreatedAt: inv.CreatedAt,
	}
}

type permissionsJSON struct {
	TeamID               uuid.UUID `json:"team_id"`
	Role                 string    `json:"role"`
	CanInvite            bool      `json:"can_invite"`
	CanManageInvitations bool      `json:"can_manage_invitations"`
	CanRemoveMembers     bool      `json:"can_remove_members"`
	CanChangeRoles       bool      `json:"can_change_roles"`
	CanLeave             bool      `json:"can_leave"`
}

func (h *Handler) identity(w http.ResponseWriter, r *http.Request) (reqctx.Identity, bool) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
	}
	return identity, ok
}

func (h *Handler) teamID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	return httpx.ParseUUIDParam(w, r, h.logger, "teamID")
}

type createTeamRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) HandleCreateTeam(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	var req createTeamRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	summary, err := h.svc.CreateTeam(r.Context(), identity, req.Name, req.Description)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	item := toTeamJSON(summary.Team)
	item.MyRole = summary.MyRole
	httpx.Data(w, http.StatusCreated, item)
}

func (h *Handler) HandleListMyTeams(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	summaries, err := h.svc.ListMyTeams(r.Context(), identity)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	items := make([]teamJSON, 0, len(summaries))
	for _, s := range summaries {
		item := toTeamJSON(s.Team)
		item.MyRole = s.MyRole
		items = append(items, item)
	}
	httpx.Data(w, http.StatusOK, items)
}

func (h *Handler) HandleGetTeam(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teamID, ok := h.teamID(w, r)
	if !ok {
		return
	}
	summary, err := h.svc.GetTeam(r.Context(), identity, teamID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	item := toTeamJSON(summary.Team)
	item.MyRole = summary.MyRole
	httpx.Data(w, http.StatusOK, item)
}

func (h *Handler) HandleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teamID, ok := h.teamID(w, r)
	if !ok {
		return
	}
	var req createTeamRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	summary, err := h.svc.UpdateTeam(r.Context(), identity, teamID, req.Name, req.Description)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	item := toTeamJSON(summary.Team)
	item.MyRole = summary.MyRole
	httpx.Data(w, http.StatusOK, item)
}

func (h *Handler) HandlePermissions(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teamID, ok := h.teamID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Permissions(r.Context(), identity, teamID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, permissionsJSON{
		TeamID: p.TeamID, Role: p.Role, CanInvite: p.CanInvite, CanManageInvitations: p.CanManageInvitations,
		CanRemoveMembers: p.CanRemoveMembers, CanChangeRoles: p.CanChangeRoles, CanLeave: p.CanLeave,
	})
}

func (h *Handler) HandleListMembers(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teamID, ok := h.teamID(w, r)
	if !ok {
		return
	}
	members, err := h.svc.ListMembers(r.Context(), identity, teamID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	items := make([]memberJSON, 0, len(members))
	for _, m := range members {
		items = append(items, memberJSON{UserID: m.UserID, Username: m.Username, DisplayName: m.DisplayName, Role: m.Role, JoinedAt: m.JoinedAt})
	}
	httpx.Data(w, http.StatusOK, items)
}

type inviteRequest struct {
	Username       string `json:"username"`
	Role           string `json:"role"`
	ExpiresInHours int    `json:"expires_in_hours"`
}

func (h *Handler) HandleInvite(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teamID, ok := h.teamID(w, r)
	if !ok {
		return
	}
	var req inviteRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	inv, err := h.svc.Invite(r.Context(), identity, teamID, req.Username, req.Role, req.ExpiresInHours)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusCreated, toInvitationJSON(inv))
}

func (h *Handler) HandleListTeamInvitations(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teamID, ok := h.teamID(w, r)
	if !ok {
		return
	}
	invitations, err := h.svc.ListTeamInvitations(r.Context(), identity, teamID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	items := make([]invitationJSON, 0, len(invitations))
	for _, inv := range invitations {
		items = append(items, toInvitationJSON(inv))
	}
	httpx.Data(w, http.StatusOK, items)
}

func (h *Handler) HandleCancelInvitation(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teamID, ok := h.teamID(w, r)
	if !ok {
		return
	}
	invitationID, ok := httpx.ParseUUIDParam(w, r, h.logger, "invitationID")
	if !ok {
		return
	}
	if err := h.svc.CancelInvitation(r.Context(), identity, teamID, invitationID); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, map[string]bool{"revoked": true})
}

type changeRoleRequest struct {
	Role string `json:"role"`
}

func (h *Handler) HandleChangeMemberRole(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teamID, ok := h.teamID(w, r)
	if !ok {
		return
	}
	userID, ok := httpx.ParseUUIDParam(w, r, h.logger, "userID")
	if !ok {
		return
	}
	var req changeRoleRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	if err := h.svc.ChangeMemberRole(r.Context(), identity, teamID, userID, req.Role); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, map[string]any{"user_id": userID, "team_id": teamID, "role": req.Role})
}

func (h *Handler) HandleRemoveMember(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teamID, ok := h.teamID(w, r)
	if !ok {
		return
	}
	userID, ok := httpx.ParseUUIDParam(w, r, h.logger, "userID")
	if !ok {
		return
	}
	if err := h.svc.RemoveMember(r.Context(), identity, teamID, userID); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, map[string]bool{"removed": true})
}

func (h *Handler) HandleListMyInvitations(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	invitations, err := h.svc.ListMyInvitations(r.Context(), identity)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	items := make([]invitationJSON, 0, len(invitations))
	for _, inv := range invitations {
		items = append(items, toInvitationJSON(inv))
	}
	httpx.Data(w, http.StatusOK, items)
}

func (h *Handler) HandleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	invitationID, ok := httpx.ParseUUIDParam(w, r, h.logger, "invitationID")
	if !ok {
		return
	}
	inv, err := h.svc.AcceptInvitation(r.Context(), identity, invitationID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, toInvitationJSON(inv))
}

func (h *Handler) HandleDeclineInvitation(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	invitationID, ok := httpx.ParseUUIDParam(w, r, h.logger, "invitationID")
	if !ok {
		return
	}
	if err := h.svc.DeclineInvitation(r.Context(), identity, invitationID); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.Data(w, http.StatusOK, map[string]string{"status": InvitationDeclined})
}

func (h *Handler) HandleListAllTeams(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}
	teams, err := h.svc.ListAllTeams(r.Context(), identity)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	items := make([]teamJSON, 0, len(teams))
	for _, t := range teams {
		items = append(items, toTeamJSON(t))
	}
	httpx.Data(w, http.StatusOK, items)
}
