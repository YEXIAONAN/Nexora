package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

// Handler maps HTTP requests to the auth service. It contains no business
// rules beyond transport-level parsing and response shaping.
type Handler struct {
	svc          *Service
	cookieSecure bool
	sessionTTL   time.Duration
	logger       *slog.Logger
}

func NewHandler(svc *Service, cookieSecure bool, sessionTTL time.Duration, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, cookieSecure: cookieSecure, sessionTTL: sessionTTL, logger: logger}
}

// PublicRoutes registers endpoints reachable without a session.
func (h *Handler) PublicRoutes(r chi.Router) {
	r.Post("/auth/login", h.HandleLogin)
	r.Post("/auth/register", h.HandleRegister)
}

// AuthenticatedRoutes registers endpoints that require a valid session.
// Platform-level authorization is enforced inside the service.
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	r.Get("/auth/me", h.HandleMe)
	r.Patch("/auth/me", h.HandleUpdateProfile)
	r.Post("/auth/me/password", h.HandleChangePassword)
	r.Delete("/auth/session", h.HandleLogout)
	r.Get("/admin/users", h.HandleListUsers)
	r.Post("/admin/users", h.HandleCreateUser)
	r.Patch("/admin/users/{userID}", h.HandleUpdateUser)
	r.Delete("/admin/users/{userID}/sessions", h.HandleRevokeUserSessions)
	r.Post("/admin/platform-roles", h.HandleGrantPlatformRole)
	r.Delete("/admin/platform-roles/{userID}", h.HandleRevokePlatformRole)
}

type publicUser struct {
	ID            uuid.UUID `json:"id"`
	Username      string    `json:"username"`
	DisplayName   string    `json:"display_name"`
	Email         string    `json:"email"`
	SystemRoot    bool      `json:"system_root"`
	PlatformAdmin bool      `json:"platform_admin"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
}

func toPublicUser(u User, platformAdmin bool) publicUser {
	return publicUser{
		ID:            u.ID,
		Username:      u.Username,
		DisplayName:   u.DisplayName,
		Email:         u.Email,
		SystemRoot:    u.SystemRoot,
		PlatformAdmin: platformAdmin,
		IsActive:      u.IsActive,
		CreatedAt:     u.CreatedAt,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	User      publicUser `json:"user"`
	ExpiresAt time.Time  `json:"expires_at"`
}

func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	fields := make(map[string]string)
	if req.Username == "" {
		fields["username"] = "is required"
	}
	if req.Password == "" {
		fields["password"] = "is required"
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, h.logger, httpx.ValidationFailed(fields))
		return
	}

	result, err := h.svc.Login(r.Context(), req.Username, req.Password, r.UserAgent(), clientIP(r))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	h.setSessionCookie(w, result.Token)
	httpx.Data(w, http.StatusOK, loginResponse{
		User:      toPublicUser(result.User, result.PlatformAdmin),
		ExpiresAt: result.ExpiresAt,
	})
}

type registerRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	user, err := h.svc.Register(r.Context(), req.Username, req.DisplayName, req.Email, req.Password)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	httpx.Data(w, http.StatusCreated, toPublicUser(user, false))
}

func (h *Handler) HandleMe(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	user, err := h.svc.repo.UserByID(r.Context(), h.svc.repo.pool, identity.UserID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, httpx.Internal(err))
		return
	}
	httpx.Data(w, http.StatusOK, toPublicUser(user, identity.PlatformAdmin))
}

type updateProfileRequest struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func (h *Handler) HandleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	var req updateProfileRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	user, err := h.svc.UpdateProfile(r.Context(), identity, req.DisplayName, req.Email)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	httpx.Data(w, http.StatusOK, toPublicUser(user, identity.PlatformAdmin))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *Handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	var req changePasswordRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	if err := h.svc.ChangePassword(r.Context(), identity, req.CurrentPassword, req.NewPassword); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	h.clearSessionCookie(w)
	httpx.Data(w, http.StatusOK, map[string]bool{"changed": true})
}

func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	if err := h.svc.Logout(r.Context(), identity); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	h.clearSessionCookie(w)
	httpx.Data(w, http.StatusOK, map[string]bool{"revoked": true})
}

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

func (h *Handler) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	var req createUserRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	user, err := h.svc.CreateUserAccount(r.Context(), identity, req.Username, req.DisplayName, req.Email, req.Password)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	httpx.Data(w, http.StatusCreated, toPublicUser(user, false))
}

type usersPage struct {
	Items  []publicUser `json:"items"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	limit := 25
	offset := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &limit); err != nil {
			limit = 25
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &offset); err != nil {
			offset = 0
		}
	}
	users, total, err := h.svc.ListUsers(r.Context(), identity, r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	items := make([]publicUser, 0, len(users))
	for _, u := range users {
		items = append(items, toPublicUser(u, u.PlatformAdmin))
	}
	httpx.Data(w, http.StatusOK, usersPage{Items: items, Total: total, Limit: limit, Offset: offset})
}

type updateUserRequest struct {
	DisplayName *string `json:"display_name"`
	Email       *string `json:"email"`
	IsActive    *bool   `json:"is_active"`
}

func (h *Handler) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	targetID, ok := httpx.ParseUUIDParam(w, r, h.logger, "userID")
	if !ok {
		return
	}
	var req updateUserRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	user, err := h.svc.UpdateUser(r.Context(), identity, targetID, AdminUserUpdate{
		DisplayName: req.DisplayName,
		Email:       req.Email,
		IsActive:    req.IsActive,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	httpx.Data(w, http.StatusOK, toPublicUser(user, user.PlatformAdmin))
}

func (h *Handler) HandleRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	targetID, ok := httpx.ParseUUIDParam(w, r, h.logger, "userID")
	if !ok {
		return
	}
	count, err := h.svc.RevokeUserSessions(r.Context(), identity, targetID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	httpx.Data(w, http.StatusOK, map[string]int64{"revoked_sessions": count})
}

type grantPlatformRoleRequest struct {
	UserID string `json:"user_id"`
}

func (h *Handler) HandleGrantPlatformRole(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	var req grantPlatformRoleRequest
	if !httpx.DecodeBody(w, r, h.logger, &req) {
		return
	}
	targetID, err := uuid.Parse(req.UserID)
	if err != nil {
		httpx.WriteError(w, r, h.logger, httpx.ValidationFailed(map[string]string{"user_id": "must be a valid UUID"}))
		return
	}
	if err := h.svc.GrantPlatformRole(r.Context(), identity, targetID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	httpx.Data(w, http.StatusCreated, map[string]string{"user_id": targetID.String(), "role": PlatformRoleAdmin})
}

func (h *Handler) HandleRevokePlatformRole(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	targetID, ok := httpx.ParseUUIDParam(w, r, h.logger, "userID")
	if !ok {
		return
	}
	if err := h.svc.RevokePlatformRole(r.Context(), identity, targetID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	httpx.Data(w, http.StatusOK, map[string]bool{"revoked": true})
}

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("invalid username or password"))
	case errors.Is(err, ErrRootAlreadyInitialized):
		httpx.WriteError(w, r, h.logger, httpx.Conflict("ROOT is already initialized"))
	case errors.Is(err, ErrUsernameTaken):
		httpx.WriteError(w, r, h.logger, httpx.Conflict("the username is already taken"))
	case errors.Is(err, ErrEmailTaken):
		httpx.WriteError(w, r, h.logger, httpx.Conflict("the email is already in use"))
	case errors.Is(err, ErrAccountDisabled):
		httpx.WriteError(w, r, h.logger, httpx.Forbidden("this account has been disabled"))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.sessionTTL.Seconds()),
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
