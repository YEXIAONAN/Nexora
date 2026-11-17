package teams

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Repository owns all SQL for teams, memberships and invitations. Every
// membership query is constrained by both team_id and user_id so a resource
// from another team can never be loaded and filtered in memory.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const teamColumns = `id, name, description, created_by, created_at, updated_at`

func scanTeam(row pgx.Row) (Team, error) {
	var t Team
	err := row.Scan(&t.ID, &t.Name, &t.Description, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, ErrNotFound
	}
	if err != nil {
		return Team{}, fmt.Errorf("query team: %w", err)
	}
	return t, nil
}

func (r *Repository) InsertTeam(ctx context.Context, tx pgx.Tx, t *Team) error {
	err := tx.QueryRow(ctx, `
		INSERT INTO teams (id, name, description, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at, updated_at`,
		t.ID, t.Name, t.Description, t.CreatedBy).Scan(&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert team: %w", err)
	}
	return nil
}

func (r *Repository) TeamByID(ctx context.Context, q querier, id uuid.UUID) (Team, error) {
	return scanTeam(q.QueryRow(ctx, `SELECT `+teamColumns+` FROM teams WHERE id = $1`, id))
}

// LockTeam serializes all member and invitation mutations of one team. It
// must be the first lock taken in any mutation transaction.
func (r *Repository) LockTeam(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var locked uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM teams WHERE id = $1 FOR UPDATE`, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock team: %w", err)
	}
	return nil
}

const memberColumns = `m.team_id, m.user_id, u.username, u.display_name, m.role, m.created_at`

func scanMember(row pgx.Row) (Member, error) {
	var m Member
	err := row.Scan(&m.TeamID, &m.UserID, &m.Username, &m.DisplayName, &m.Role, &m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	if err != nil {
		return Member{}, fmt.Errorf("query member: %w", err)
	}
	return m, nil
}

// Membership resolves only active memberships of the given team and user.
func (r *Repository) Membership(ctx context.Context, q querier, teamID, userID uuid.UUID) (Member, error) {
	return scanMember(q.QueryRow(ctx, `
		SELECT `+memberColumns+`
		FROM team_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.team_id = $1 AND m.user_id = $2 AND m.status = 'active'`,
		teamID, userID))
}

func (r *Repository) InsertMember(ctx context.Context, tx pgx.Tx, teamID, userID uuid.UUID, role string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)`,
		teamID, userID, role)
	if err != nil {
		return fmt.Errorf("insert member: %w", err)
	}
	return nil
}

// ActivateMember inserts a membership or reactivates a removed one. The
// composite primary key prevents duplicate memberships under concurrency.
func (r *Repository) ActivateMember(ctx context.Context, tx pgx.Tx, teamID, userID uuid.UUID, role string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (team_id, user_id) DO UPDATE
		SET role = EXCLUDED.role, status = 'active', removed_at = NULL, updated_at = now()`,
		teamID, userID, role)
	if err != nil {
		return fmt.Errorf("activate member: %w", mapMemberMutationError(err))
	}
	return nil
}

func (r *Repository) UpdateMemberRole(ctx context.Context, tx pgx.Tx, teamID, userID uuid.UUID, role string) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE team_members SET role = $3, updated_at = now()
		WHERE team_id = $1 AND user_id = $2 AND status = 'active'`,
		teamID, userID, role)
	if err != nil {
		return false, fmt.Errorf("update member role: %w", mapMemberMutationError(err))
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) RemoveMember(ctx context.Context, tx pgx.Tx, teamID, userID uuid.UUID) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE team_members SET status = 'removed', removed_at = now(), updated_at = now()
		WHERE team_id = $1 AND user_id = $2 AND status = 'active'`,
		teamID, userID)
	if err != nil {
		return false, fmt.Errorf("remove member: %w", mapMemberMutationError(err))
	}
	return tag.RowsAffected() > 0, nil
}

// mapMemberMutationError translates the last-owner trigger exception into a
// domain error; every other failure stays wrapped for internal handling.
func mapMemberMutationError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "P0001" && strings.Contains(pgErr.Message, "active owner") {
		return ErrLastOwner
	}
	return err
}

func (r *Repository) ListMembers(ctx context.Context, q querier, teamID uuid.UUID) ([]Member, error) {
	rows, err := q.Query(ctx, `
		SELECT `+memberColumns+`
		FROM team_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.team_id = $1 AND m.status = 'active'
		ORDER BY m.created_at, u.username`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	var members []Member
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

func (r *Repository) ListMyTeams(ctx context.Context, q querier, userID uuid.UUID) ([]TeamSummary, error) {
	rows, err := q.Query(ctx, `
		SELECT t.id, t.name, t.description, t.created_by, t.created_at, t.updated_at, m.role
		FROM teams t
		JOIN team_members m ON m.team_id = t.id
		WHERE m.user_id = $1 AND m.status = 'active'
		ORDER BY t.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("list my teams: %w", err)
	}
	defer rows.Close()
	var summaries []TeamSummary
	for rows.Next() {
		var s TeamSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt, &s.MyRole); err != nil {
			return nil, fmt.Errorf("list my teams: %w", err)
		}
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list my teams: %w", err)
	}
	return summaries, nil
}

func (r *Repository) ListAllTeams(ctx context.Context, q querier) ([]Team, error) {
	rows, err := q.Query(ctx, `SELECT `+teamColumns+` FROM teams ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list all teams: %w", err)
	}
	defer rows.Close()
	var teams []Team
	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list all teams: %w", err)
	}
	return teams, nil
}

const invitationColumns = `i.id, i.team_id, t.name, i.invited_user_id, u.username, i.invited_by, i.role, i.status, i.expires_at, i.created_at`

const invitationFrom = `
	FROM team_invitations i
	JOIN teams t ON t.id = i.team_id
	JOIN users u ON u.id = i.invited_user_id`

func scanInvitation(row pgx.Row) (Invitation, error) {
	var inv Invitation
	err := row.Scan(&inv.ID, &inv.TeamID, &inv.TeamName, &inv.InvitedUserID, &inv.InvitedUsername,
		&inv.InvitedBy, &inv.Role, &inv.Status, &inv.ExpiresAt, &inv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrNotFound
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("query invitation: %w", err)
	}
	return inv, nil
}

func (r *Repository) InsertInvitation(ctx context.Context, tx pgx.Tx, inv Invitation) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO team_invitations (id, team_id, invited_user_id, invited_by, role, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		inv.ID, inv.TeamID, inv.InvitedUserID, inv.InvitedBy, inv.Role, inv.ExpiresAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "team_invitations_one_pending" {
		return ErrDuplicateInvitation
	}
	if err != nil {
		return fmt.Errorf("insert invitation: %w", err)
	}
	return nil
}

func (r *Repository) InvitationByID(ctx context.Context, q querier, id uuid.UUID) (Invitation, error) {
	return scanInvitation(q.QueryRow(ctx, `SELECT `+invitationColumns+invitationFrom+` WHERE i.id = $1`, id))
}

// LockInvitation locks only the invitation row; the caller must already hold
// the team lock so lock ordering stays team-first everywhere.
func (r *Repository) LockInvitation(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Invitation, error) {
	return scanInvitation(tx.QueryRow(ctx, `SELECT `+invitationColumns+invitationFrom+` WHERE i.id = $1 FOR UPDATE OF i`, id))
}

func (r *Repository) SetInvitationStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status string) error {
	_, err := tx.Exec(ctx, `
		UPDATE team_invitations SET status = $2, responded_at = now(), updated_at = now()
		WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("update invitation status: %w", err)
	}
	return nil
}

func (r *Repository) listInvitations(ctx context.Context, q querier, where string, args ...any) ([]Invitation, error) {
	rows, err := q.Query(ctx, `SELECT `+invitationColumns+invitationFrom+` WHERE `+where+` ORDER BY i.created_at DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()
	var invitations []Invitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		invitations = append(invitations, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	return invitations, nil
}

func (r *Repository) ListTeamInvitations(ctx context.Context, q querier, teamID uuid.UUID) ([]Invitation, error) {
	return r.listInvitations(ctx, q, `i.team_id = $1`, teamID)
}

// ListMyInvitations returns only live pending invitations of the user.
func (r *Repository) ListMyInvitations(ctx context.Context, q querier, userID uuid.UUID) ([]Invitation, error) {
	return r.listInvitations(ctx, q, `i.invited_user_id = $1 AND i.status = 'pending' AND i.expires_at > now()`, userID)
}
