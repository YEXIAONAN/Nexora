package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User is an account row. PasswordHash never leaves the service layer.
type User struct {
	ID           uuid.UUID
	Username     string
	DisplayName  string
	PasswordHash string
	SystemRoot   bool
	CreatedAt    time.Time
}

// Session is a server-side session row; only the token digest is stored.
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	UserAgent string
	ExpiresAt time.Time
}

// SessionIdentity is the per-request authentication result: a valid session
// joined with its account and platform role in a single query.
type SessionIdentity struct {
	SessionID     uuid.UUID
	UserID        uuid.UUID
	Username      string
	DisplayName   string
	SystemRoot    bool
	PlatformAdmin bool
	ExpiresAt     time.Time
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository owns all SQL for users, sessions and platform roles.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const userColumns = `id, username, display_name, password_hash, system_root, created_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &u.SystemRoot, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

func (r *Repository) UserByUsername(ctx context.Context, q querier, username string) (User, error) {
	return scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE username = $1`, username))
}

func (r *Repository) UserByID(ctx context.Context, q querier, id uuid.UUID) (User, error) {
	return scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func (r *Repository) RootExists(ctx context.Context, q querier) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE system_root)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check root existence: %w", err)
	}
	return exists, nil
}

// InsertUser maps unique violations to domain errors. The partial unique
// index users_single_system_root is what makes concurrent bootstrap safe.
func (r *Repository) InsertUser(ctx context.Context, tx pgx.Tx, u User) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, display_name, password_hash, system_root)
		VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Username, u.DisplayName, u.PasswordHash, u.SystemRoot)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_single_system_root":
			return ErrRootAlreadyInitialized
		case "users_username_key":
			return ErrUsernameTaken
		}
	}
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *Repository) InsertSession(ctx context.Context, tx pgx.Tx, s Session) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5)`,
		s.ID, s.UserID, s.TokenHash, s.UserAgent, s.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// SessionIdentityByTokenHash resolves only live sessions; expired or revoked
// rows are indistinguishable from unknown tokens.
func (r *Repository) SessionIdentityByTokenHash(ctx context.Context, q querier, tokenHash string) (SessionIdentity, error) {
	var s SessionIdentity
	err := q.QueryRow(ctx, `
		SELECT s.id, s.user_id, u.username, u.display_name, u.system_root,
		       (pr.user_id IS NOT NULL) AS platform_admin, s.expires_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN platform_roles pr ON pr.user_id = u.id AND pr.role = 'platform_admin'
		WHERE s.token_hash = $1 AND s.expires_at > now() AND s.revoked_at IS NULL`,
		tokenHash).Scan(&s.SessionID, &s.UserID, &s.Username, &s.DisplayName, &s.SystemRoot, &s.PlatformAdmin, &s.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionIdentity{}, ErrNotFound
	}
	if err != nil {
		return SessionIdentity{}, fmt.Errorf("query session: %w", err)
	}
	return s, nil
}

func (r *Repository) RevokeSession(ctx context.Context, tx pgx.Tx, id uuid.UUID) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return false, fmt.Errorf("revoke session: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) RevokeUserSessions(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (int64, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return 0, fmt.Errorf("revoke user sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *Repository) HasPlatformRole(ctx context.Context, q querier, userID uuid.UUID, role string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM platform_roles WHERE user_id = $1 AND role = $2)`,
		userID, role).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check platform role: %w", err)
	}
	return exists, nil
}

func (r *Repository) InsertPlatformRole(ctx context.Context, tx pgx.Tx, userID uuid.UUID, role string, grantedBy uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO platform_roles (user_id, role, granted_by)
		VALUES ($1, $2, $3)`, userID, role, grantedBy)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "platform_roles_pkey" {
		return ErrRoleAlreadyGranted
	}
	if err != nil {
		return fmt.Errorf("insert platform role: %w", err)
	}
	return nil
}

func (r *Repository) DeletePlatformRole(ctx context.Context, tx pgx.Tx, userID uuid.UUID, role string) (bool, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM platform_roles WHERE user_id = $1 AND role = $2`, userID, role)
	if err != nil {
		return false, fmt.Errorf("delete platform role: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
