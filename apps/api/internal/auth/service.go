package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

const (
	// SessionCookieName is host-only, HttpOnly and never readable by scripts.
	SessionCookieName = "nexora_session"
	sessionTokenBytes = 32
	// PlatformRoleAdmin is the only platform role in Phase 02.
	PlatformRoleAdmin = "platform_admin"
)

// RegistrationMode controls how new accounts can be created on this
// installation. Development defaults to open; production defaults to invite,
// where a pending team invitation is required for the chosen username.
type RegistrationMode string

const (
	RegistrationOpen   RegistrationMode = "open"
	RegistrationInvite RegistrationMode = "invite"
	RegistrationClosed RegistrationMode = "closed"
)

// InvitationChecker is implemented by the teams module so registration can
// honour invite-only mode without auth reading team tables directly.
type InvitationChecker interface {
	HasPendingInvitationForUsername(ctx context.Context, username string) (bool, error)
}

// Service implements authentication, session lifecycle, ROOT bootstrap and
// platform role management. Authorization for team resources lives in the
// teams module; this service never grants team access by itself.
type Service struct {
	repo             *Repository
	audit            *audit.Repository
	limiter          *LoginLimiter
	sessionTTL       time.Duration
	logger           *slog.Logger
	registrationMode RegistrationMode
	invitations      InvitationChecker
}

func NewService(repo *Repository, auditRepo *audit.Repository, limiter *LoginLimiter, sessionTTL time.Duration, logger *slog.Logger) *Service {
	return &Service{repo: repo, audit: auditRepo, limiter: limiter, sessionTTL: sessionTTL, logger: logger, registrationMode: RegistrationOpen}
}

// SetRegistrationMode configures the public sign-up policy at startup.
func (s *Service) SetRegistrationMode(mode RegistrationMode) { s.registrationMode = mode }

// SetInvitationChecker enables invite-only registration.
func (s *Service) SetInvitationChecker(checker InvitationChecker) { s.invitations = checker }

// RegistrationMode exposes the effective policy to the meta endpoint.
func (s *Service) RegistrationMode() RegistrationMode { return s.registrationMode }

// LoginResult carries the raw token exactly once, for the Set-Cookie header.
// It is never logged, audited or persisted.
type LoginResult struct {
	Token         string
	User          User
	SessionID     uuid.UUID
	ExpiresAt     time.Time
	PlatformAdmin bool
}

func newSessionToken() (token, tokenHash string, err error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) event(ctx context.Context, actorID *uuid.UUID, action, targetType string, targetID *uuid.UUID, details map[string]any) audit.Event {
	return audit.Event{
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		RequestID:  reqctx.RequestID(ctx),
		Details:    details,
	}
}

func (s *Service) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.repo.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// Login verifies credentials, applies the failure rate limit and always issues
// a fresh session token (session fixation protection).
func (s *Service) Login(ctx context.Context, username, password, userAgent, clientIP string) (LoginResult, error) {
	normalized := strings.ToLower(strings.TrimSpace(username))
	key := clientIP + "|" + normalized

	if !s.limiter.Allow(key) {
		s.recordLoginFailure(ctx, normalized, "rate_limited")
		return LoginResult{}, httpx.TooManyRequests("too many failed login attempts; try again later")
	}

	user, err := s.repo.UserByUsername(ctx, s.repo.pool, normalized)
	switch {
	case errors.Is(err, ErrNotFound):
		VerifyAgainstDummy(password)
		s.limiter.RecordFailure(key)
		s.recordLoginFailure(ctx, normalized, "invalid_credentials")
		return LoginResult{}, ErrInvalidCredentials
	case err != nil:
		return LoginResult{}, httpx.Internal(err)
	}

	if !VerifyPassword(user.PasswordHash, password) {
		s.limiter.RecordFailure(key)
		s.recordLoginFailure(ctx, normalized, "invalid_credentials")
		return LoginResult{}, ErrInvalidCredentials
	}
	if !user.IsActive {
		s.recordLoginFailure(ctx, normalized, "account_disabled")
		return LoginResult{}, ErrAccountDisabled
	}
	s.limiter.Reset(key)

	token, tokenHash, err := newSessionToken()
	if err != nil {
		return LoginResult{}, httpx.Internal(err)
	}
	session := Session{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		UserAgent: truncate(userAgent, 256),
		ExpiresAt: time.Now().UTC().Add(s.sessionTTL),
	}
	var platformAdmin bool
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.InsertSession(ctx, tx, session); err != nil {
			return httpx.Internal(err)
		}
		var err error
		platformAdmin, err = s.repo.HasPlatformRole(ctx, tx, user.ID, PlatformRoleAdmin)
		if err != nil {
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &user.ID, "auth.login_success", "user", &user.ID, map[string]any{"username": user.Username}))
	})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, User: user, SessionID: session.ID, ExpiresAt: session.ExpiresAt, PlatformAdmin: platformAdmin}, nil
}

// recordLoginFailure is best effort: an audit outage must not change the
// authentication outcome. Successful logins audit inside the session tx.
func (s *Service) recordLoginFailure(ctx context.Context, username, reason string) {
	ev := s.event(ctx, nil, "auth.login_failed", "", nil, map[string]any{"username": username, "reason": reason})
	if err := s.audit.InsertStandalone(ctx, ev); err != nil {
		s.logger.Warn("audit: login failure event not recorded", "error", err)
	}
}

// Authenticate resolves a raw session token to the request identity. Team
// membership is intentionally not part of this result.
func (s *Service) Authenticate(ctx context.Context, token string) (reqctx.Identity, error) {
	if token == "" {
		return reqctx.Identity{}, ErrNotFound
	}
	row, err := s.repo.SessionIdentityByTokenHash(ctx, s.repo.pool, hashToken(token))
	if err != nil {
		return reqctx.Identity{}, err
	}
	return reqctx.Identity{
		UserID:        row.UserID,
		SessionID:     row.SessionID,
		Username:      row.Username,
		DisplayName:   row.DisplayName,
		SystemRoot:    row.SystemRoot,
		PlatformAdmin: row.PlatformAdmin,
	}, nil
}

// Logout revokes the current session server-side.
func (s *Service) Logout(ctx context.Context, identity reqctx.Identity) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.repo.RevokeSession(ctx, tx, identity.SessionID); err != nil {
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &identity.UserID, "auth.logout", "session", &identity.SessionID, nil))
	})
}

// BootstrapRoot creates the single immutable ROOT identity. Concurrency is
// safe through the partial unique index: the losing transaction receives
// ErrRootAlreadyInitialized instead of creating a second ROOT.
func (s *Service) BootstrapRoot(ctx context.Context, username, password string) (User, error) {
	normalized := strings.ToLower(strings.TrimSpace(username))
	if fields := validateAccountFields(normalized, "", password); len(fields) > 0 {
		return User{}, httpx.ValidationFailed(fields)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, httpx.Internal(err)
	}
	user := User{ID: uuid.New(), Username: normalized, DisplayName: normalized, PasswordHash: hash, SystemRoot: true, IsActive: true}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		exists, err := s.repo.RootExists(ctx, tx)
		if err != nil {
			return httpx.Internal(err)
		}
		if exists {
			return ErrRootAlreadyInitialized
		}
		if err := s.repo.InsertUser(ctx, tx, user); err != nil {
			return err
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &user.ID, "root.bootstrap", "user", &user.ID, map[string]any{"username": user.Username}))
	})
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// Account is the minimal account view shared with other modules (for example
// team invitations). It never carries password hashes or session data.
type Account struct {
	ID          uuid.UUID
	Username    string
	DisplayName string
}

// AccountByUsername resolves an existing account for cross-module lookups.
// Usernames are normalized the same way as at login.
func (s *Service) AccountByUsername(ctx context.Context, username string) (Account, error) {
	u, err := s.repo.UserByUsername(ctx, s.repo.pool, strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return Account{}, err
	}
	return Account{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName}, nil
}

// RootInitialized reports whether the installation already has its ROOT.
func (s *Service) RootInitialized(ctx context.Context) (bool, error) {
	return s.repo.RootExists(ctx, s.repo.pool)
}

// CreateUser is the authorized account-creation channel (ROOT or platform
// admin). There is no public registration endpoint.
func (s *Service) CreateUser(ctx context.Context, actor reqctx.Identity, username, displayName, password string) (User, error) {
	return s.CreateUserAccount(ctx, actor, username, displayName, "", password)
}

// CreateUserAccount creates an active account with an optional email.
func (s *Service) CreateUserAccount(ctx context.Context, actor reqctx.Identity, username, displayName, email, password string) (User, error) {
	if !actor.SystemRoot && !actor.PlatformAdmin {
		return User{}, httpx.Forbidden("only platform administrators may create accounts")
	}
	normalized := strings.ToLower(strings.TrimSpace(username))
	email = normalizeEmail(email)
	if fields := validateAccountFields(normalized, displayName, password); len(fields) > 0 {
		return User{}, httpx.ValidationFailed(fields)
	}
	if msg := EmailFieldError(email); msg != "" {
		return User{}, httpx.ValidationFailed(map[string]string{"email": msg})
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, httpx.Internal(err)
	}
	user := User{ID: uuid.New(), Username: normalized, DisplayName: displayName, Email: email, PasswordHash: hash, IsActive: true}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.InsertUser(ctx, tx, user); err != nil {
			switch {
			case errors.Is(err, ErrUsernameTaken):
				return httpx.Conflict("the username is already taken")
			case errors.Is(err, ErrEmailTaken):
				return httpx.Conflict("the email is already in use")
			}
			return httpx.Internal(err)
		}
		stored, err := s.repo.UserByID(ctx, tx, user.ID)
		if err != nil {
			return httpx.Internal(err)
		}
		user = stored
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "user.created", "user", &user.ID, map[string]any{"username": user.Username}))
	})
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// Register creates a normal account through the public sign-up policy. It
// never grants platform or team authority; a pending invitation is claimed
// later by username when the invitee opens the invitation inbox.
func (s *Service) Register(ctx context.Context, username, displayName, email, password string) (User, error) {
	if s.registrationMode == RegistrationClosed {
		return User{}, httpx.Forbidden("public registration is disabled on this installation")
	}
	normalized := strings.ToLower(strings.TrimSpace(username))
	if s.registrationMode == RegistrationInvite {
		if s.invitations == nil {
			return User{}, httpx.Forbidden("registration requires a team invitation")
		}
		allowed, err := s.invitations.HasPendingInvitationForUsername(ctx, normalized)
		if err != nil {
			return User{}, httpx.Internal(err)
		}
		if !allowed {
			return User{}, httpx.Forbidden("registration requires a pending team invitation for this username")
		}
	}
	email = normalizeEmail(email)
	if fields := validateAccountFields(normalized, displayName, password); len(fields) > 0 {
		return User{}, httpx.ValidationFailed(fields)
	}
	if msg := EmailFieldError(email); msg != "" {
		return User{}, httpx.ValidationFailed(map[string]string{"email": msg})
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, httpx.Internal(err)
	}
	user := User{ID: uuid.New(), Username: normalized, DisplayName: displayName, Email: email, PasswordHash: hash, IsActive: true}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.InsertUser(ctx, tx, user); err != nil {
			switch {
			case errors.Is(err, ErrUsernameTaken):
				return httpx.Conflict("the username is already taken")
			case errors.Is(err, ErrEmailTaken):
				return httpx.Conflict("the email is already in use")
			}
			return httpx.Internal(err)
		}
		stored, err := s.repo.UserByID(ctx, tx, user.ID)
		if err != nil {
			return httpx.Internal(err)
		}
		user = stored
		return s.audit.Insert(ctx, tx, s.event(ctx, &user.ID, "user.registered", "user", &user.ID, map[string]any{"username": user.Username}))
	})
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// UpdateProfile lets an authenticated user change their own display name and
// email. Username, roles and account status remain administrator-controlled.
func (s *Service) UpdateProfile(ctx context.Context, actor reqctx.Identity, displayName, email string) (User, error) {
	email = normalizeEmail(email)
	displayName = strings.TrimSpace(displayName)
	if msg := DisplayNameFieldError(displayName); msg != "" {
		return User{}, httpx.ValidationFailed(map[string]string{"display_name": msg})
	}
	if msg := EmailFieldError(email); msg != "" {
		return User{}, httpx.ValidationFailed(map[string]string{"email": msg})
	}
	var user User
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.UpdateProfile(ctx, tx, actor.UserID, displayName, email); err != nil {
			if errors.Is(err, ErrEmailTaken) {
				return httpx.Conflict("the email is already in use")
			}
			return httpx.Internal(err)
		}
		var err error
		user, err = s.repo.UserByID(ctx, tx, actor.UserID)
		if err != nil {
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "user.profile_updated", "user", &actor.UserID, nil))
	})
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// ChangePassword verifies the current password before rotating the hash and
// revokes every other session of the account.
func (s *Service) ChangePassword(ctx context.Context, actor reqctx.Identity, currentPassword, newPassword string) error {
	fields := make(map[string]string)
	if currentPassword == "" {
		fields["current_password"] = "is required"
	}
	if msg := PasswordFieldError(actor.Username, newPassword); msg != "" {
		fields["new_password"] = msg
	}
	if len(fields) > 0 {
		return httpx.ValidationFailed(fields)
	}
	user, err := s.repo.UserByID(ctx, s.repo.pool, actor.UserID)
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("the requested user does not exist")
	}
	if err != nil {
		return httpx.Internal(err)
	}
	if !VerifyPassword(user.PasswordHash, currentPassword) {
		return httpx.ValidationFailed(map[string]string{"current_password": "is incorrect"})
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return httpx.Internal(err)
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.UpdatePassword(ctx, tx, actor.UserID, hash); err != nil {
			return httpx.Internal(err)
		}
		if _, err := s.repo.RevokeUserSessions(ctx, tx, actor.UserID); err != nil {
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "auth.password_changed", "user", &actor.UserID, nil))
	})
}

// ListUsers is the platform administration directory (ROOT or platform admin).
func (s *Service) ListUsers(ctx context.Context, actor reqctx.Identity, search string, limit, offset int) ([]User, int64, error) {
	if !actor.SystemRoot && !actor.PlatformAdmin {
		return nil, 0, httpx.Forbidden("only platform administrators may list accounts")
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	users, total, err := s.repo.ListUsers(ctx, s.repo.pool, search, limit, offset)
	if err != nil {
		return nil, 0, httpx.Internal(err)
	}
	return users, total, nil
}

// AdminUserUpdate is the administrator-editable subset of an account.
type AdminUserUpdate struct {
	DisplayName *string
	Email       *string
	IsActive    *bool
}

// UpdateUser applies administrator edits. ROOT identity flags and the actor's
// own status are protected against accidental lockout.
func (s *Service) UpdateUser(ctx context.Context, actor reqctx.Identity, targetID uuid.UUID, update AdminUserUpdate) (User, error) {
	if !actor.SystemRoot && !actor.PlatformAdmin {
		return User{}, httpx.Forbidden("only platform administrators may edit accounts")
	}
	if targetID == actor.UserID && update.IsActive != nil && !*update.IsActive {
		return User{}, httpx.Conflict("you cannot disable your own account")
	}
	var updated User
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		target, err := s.repo.UserByID(ctx, tx, targetID)
		if errors.Is(err, ErrNotFound) {
			return httpx.NotFound("the requested user does not exist")
		}
		if err != nil {
			return httpx.Internal(err)
		}
		if target.SystemRoot && !actor.SystemRoot {
			return httpx.Forbidden("only ROOT may edit the ROOT account")
		}
		displayName := target.DisplayName
		if update.DisplayName != nil {
			displayName = strings.TrimSpace(*update.DisplayName)
			if msg := DisplayNameFieldError(displayName); msg != "" {
				return httpx.ValidationFailed(map[string]string{"display_name": msg})
			}
		}
		email := target.Email
		if update.Email != nil {
			email = normalizeEmail(*update.Email)
			if msg := EmailFieldError(email); msg != "" {
				return httpx.ValidationFailed(map[string]string{"email": msg})
			}
		}
		if update.DisplayName != nil || update.Email != nil {
			if err := s.repo.UpdateProfile(ctx, tx, targetID, displayName, email); err != nil {
				if errors.Is(err, ErrEmailTaken) {
					return httpx.Conflict("the email is already in use")
				}
				return httpx.Internal(err)
			}
		}
		if update.IsActive != nil && *update.IsActive != target.IsActive {
			if target.SystemRoot && !*update.IsActive {
				return httpx.Conflict("the ROOT account cannot be disabled")
			}
			if err := s.repo.SetUserActive(ctx, tx, targetID, *update.IsActive); err != nil {
				return httpx.Internal(err)
			}
			if !*update.IsActive {
				if _, err := s.repo.RevokeUserSessions(ctx, tx, targetID); err != nil {
					return httpx.Internal(err)
				}
			}
		}
		updated, err = s.repo.UserByID(ctx, tx, targetID)
		if err != nil {
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "user.admin_updated", "user", &targetID, nil))
	})
	if err != nil {
		return User{}, err
	}
	return updated, nil
}

// GrantPlatformRole is ROOT-only. ROOT itself can never receive platform
// roles; its authority comes from the immutable system_root flag.
func (s *Service) GrantPlatformRole(ctx context.Context, actor reqctx.Identity, targetID uuid.UUID) error {
	if !actor.SystemRoot {
		return httpx.Forbidden("only ROOT may manage platform roles")
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		target, err := s.repo.UserByID(ctx, tx, targetID)
		if errors.Is(err, ErrNotFound) {
			return httpx.NotFound("the requested user does not exist")
		}
		if err != nil {
			return httpx.Internal(err)
		}
		if target.SystemRoot {
			return httpx.Conflict("the ROOT identity cannot hold platform roles")
		}
		if err := s.repo.InsertPlatformRole(ctx, tx, targetID, PlatformRoleAdmin, actor.UserID); err != nil {
			if errors.Is(err, ErrRoleAlreadyGranted) {
				return httpx.Conflict("the user already holds this platform role")
			}
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "platform_role.granted", "user", &targetID, map[string]any{"role": PlatformRoleAdmin}))
	})
}

func (s *Service) RevokePlatformRole(ctx context.Context, actor reqctx.Identity, targetID uuid.UUID) error {
	if !actor.SystemRoot {
		return httpx.Forbidden("only ROOT may manage platform roles")
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		deleted, err := s.repo.DeletePlatformRole(ctx, tx, targetID, PlatformRoleAdmin)
		if err != nil {
			return httpx.Internal(err)
		}
		if !deleted {
			return httpx.NotFound("the user does not hold this platform role")
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "platform_role.revoked", "user", &targetID, map[string]any{"role": PlatformRoleAdmin}))
	})
}

// RevokeUserSessions invalidates every live session of a user (ROOT or
// platform admin), for offboarding and incident response.
func (s *Service) RevokeUserSessions(ctx context.Context, actor reqctx.Identity, targetID uuid.UUID) (int64, error) {
	if !actor.SystemRoot && !actor.PlatformAdmin {
		return 0, httpx.Forbidden("only platform administrators may revoke user sessions")
	}
	var count int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.repo.UserByID(ctx, tx, targetID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return httpx.NotFound("the requested user does not exist")
			}
			return httpx.Internal(err)
		}
		var err error
		count, err = s.repo.RevokeUserSessions(ctx, tx, targetID)
		if err != nil {
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "auth.sessions_revoked", "user", &targetID, map[string]any{"revoked_sessions": count}))
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func validateAccountFields(username, displayName, password string) map[string]string {
	fields := make(map[string]string)
	if msg := UsernameFieldError(username); msg != "" {
		fields["username"] = msg
	}
	if msg := DisplayNameFieldError(displayName); msg != "" {
		fields["display_name"] = msg
	}
	if msg := PasswordFieldError(username, password); msg != "" {
		fields["password"] = msg
	}
	return fields
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
