package auth

import (
	"encoding/json"
	"errors"
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
}

// AuthenticatedRoutes registers endpoints that require a valid session.
// Platform-level authorization is enforced inside the service.
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	r.Get("/auth/me", h.HandleMe)
	r.Delete("/auth/session", h.HandleLogout)
	r.Post("/admin/users", h.HandleCreateUser)
	r.Delete("/admin/users/{userID}/sessions", h.HandleRevokeUserSessions)
	r.Post("/admin/platform-roles", h.HandleGrantPlatformRole)
	r.Delete("/admin/platform-roles/{userID}", h.HandleRevokePlatformRole)
}

type publicUser struct {
	ID            uuid.UUID `json:"id"`
	Username      string    `json:"username"`
	DisplayName   string    `json:"display_name"`
	SystemRoot    bool      `json:"system_root"`
	PlatformAdmin bool      `json:"platform_admin"`
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
	if !decodeBody(w, r, h.logger, &req) {
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
		User: publicUser{
			ID:            result.User.ID,
			Username:      result.User.Username,
			DisplayName:   result.User.DisplayName,
			SystemRoot:    result.User.SystemRoot,
			PlatformAdmin: result.PlatformAdmin,
		},
		ExpiresAt: result.ExpiresAt,
	})
}

func (h *Handler) HandleMe(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	httpx.Data(w, http.StatusOK, publicUser{
		ID:            identity.UserID,
		Username:      identity.Username,
		DisplayName:   identity.DisplayName,
		SystemRoot:    identity.SystemRoot,
		PlatformAdmin: identity.PlatformAdmin,
	})
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
	Password    string `json:"password"`
}

func (h *Handler) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	var req createUserRequest
	if !decodeBody(w, r, h.logger, &req) {
		return
	}
	user, err := h.svc.CreateUser(r.Context(), identity, req.Username, req.DisplayName, req.Password)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	httpx.Data(w, http.StatusCreated, publicUser{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		SystemRoot:  user.SystemRoot,
	})
}

func (h *Handler) HandleRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	identity, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
		return
	}
	targetID, ok := parseUUIDParam(w, r, h.logger, "userID")
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
	if !decodeBody(w, r, h.logger, &req) {
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
	targetID, ok := parseUUIDParam(w, r, h.logger, "userID")
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

func decodeBody(w http.ResponseWriter, r *http.Request, logger *slog.Logger, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.WriteError(w, r, logger, httpx.BadRequest("the request body must be a valid JSON object with known fields"))
		return false
	}
	return true
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, logger *slog.Logger, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(w, r, logger, httpx.BadRequest("the "+name+" path parameter must be a valid UUID"))
		return uuid.UUID{}, false
	}
	return id, true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
