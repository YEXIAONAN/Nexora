package teams

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

// AccountDirectory resolves existing accounts for invitations. It is served
// by the auth module; teams never reads or writes the users table directly.
type AccountDirectory interface {
	AccountByUsername(ctx context.Context, username string) (auth.Account, error)
}

// Service implements the team authorization chain: authenticated identity →
// team membership (single constrained SQL query) → team role capability →
// operation. ROOT does not bypass team business authorization; platform-wide
// visibility is only available through ListAllTeams.
type Service struct {
	repo     *Repository
	accounts AccountDirectory
	audit    *audit.Repository
	logger   *slog.Logger
}

func NewService(repo *Repository, accounts AccountDirectory, auditRepo *audit.Repository, logger *slog.Logger) *Service {
	return &Service{repo: repo, accounts: accounts, audit: auditRepo, logger: logger}
}

func (s *Service) event(ctx context.Context, actorID *uuid.UUID, action, targetType string, targetID, teamID *uuid.UUID, details map[string]any) audit.Event {
	return audit.Event{
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		TeamID:     teamID,
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

// denied records an authorization refusal best effort. The refusal outcome
// never depends on the audit write succeeding.
func (s *Service) denied(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID, reason string) {
	ev := s.event(ctx, &actor.UserID, "authz.denied", "team", &teamID, &teamID, map[string]any{"reason": reason})
	if err := s.audit.InsertStandalone(ctx, ev); err != nil {
		s.logger.Warn("audit: authorization denial not recorded", "error", err)
	}
}

func (s *Service) notFound(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID, reason string) error {
	s.denied(ctx, actor, teamID, reason)
	return httpx.NotFound("the requested team does not exist")
}

// refusal carries an authorization denial out of a locked transaction. The
// audit write must happen after the transaction ends: audit_events.team_id
// references teams, so inserting it from another connection while the team
// row is locked would deadlock against our own transaction.
type refusal struct {
	reason string
	err    error
}

func (r *refusal) Error() string { return r.err.Error() }

func (s *Service) unwrapRefusal(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID, err error) error {
	var ref *refusal
	if errors.As(err, &ref) {
		s.denied(ctx, actor, teamID, ref.reason)
		return ref.err
	}
	return err
}

// member resolves the actor's active membership for read paths. Non-members
// (including ROOT) receive 404 so team existence is not leaked.
func (s *Service) member(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID) (Member, error) {
	m, err := s.repo.Membership(ctx, s.repo.pool, teamID, actor.UserID)
	if errors.Is(err, ErrNotFound) {
		return Member{}, s.notFound(ctx, actor, teamID, "not_a_member")
	}
	if err != nil {
		return Member{}, httpx.Internal(err)
	}
	return m, nil
}

// mutate runs fn inside a transaction that first locks the team row and then
// re-verifies the actor's membership under that lock, so concurrent removals
// or role changes cannot slip past the authorization check.
func (s *Service) mutate(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID, fn func(tx pgx.Tx, actorMember Member) error) error {
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.LockTeam(ctx, tx, teamID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return &refusal{reason: "not_a_member", err: httpx.NotFound("the requested team does not exist")}
			}
			return httpx.Internal(err)
		}
		m, err := s.repo.Membership(ctx, tx, teamID, actor.UserID)
		if errors.Is(err, ErrNotFound) {
			return &refusal{reason: "not_a_member", err: httpx.NotFound("the requested team does not exist")}
		}
		if err != nil {
			return httpx.Internal(err)
		}
		return fn(tx, m)
	})
	return s.unwrapRefusal(ctx, actor, teamID, err)
}

func (s *Service) requireManager(actor reqctx.Identity, teamID uuid.UUID, m Member) error {
	if m.Role != RoleOwner && m.Role != RoleAdmin {
		return &refusal{reason: "insufficient_role", err: httpx.Forbidden("your team role does not allow this operation")}
	}
	return nil
}

// CreateTeam creates a team and makes the creator its first owner.
func (s *Service) CreateTeam(ctx context.Context, actor reqctx.Identity, name, description string) (TeamSummary, error) {
	name = strings.TrimSpace(name)
	fields := make(map[string]string)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxTeamNameRunes {
		fields["name"] = fmt.Sprintf("must be between 1 and %d characters", maxTeamNameRunes)
	}
	if utf8.RuneCountInString(description) > maxTeamDescriptionRunes {
		fields["description"] = fmt.Sprintf("must be at most %d characters", maxTeamDescriptionRunes)
	}
	if len(fields) > 0 {
		return TeamSummary{}, httpx.ValidationFailed(fields)
	}
	team := Team{ID: uuid.New(), Name: name, Description: description, CreatedBy: actor.UserID}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.InsertTeam(ctx, tx, &team); err != nil {
			return httpx.Internal(err)
		}
		if err := s.repo.InsertMember(ctx, tx, team.ID, actor.UserID, RoleOwner); err != nil {
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "team.created", "team", &team.ID, &team.ID, map[string]any{"name": team.Name}))
	})
	if err != nil {
		return TeamSummary{}, err
	}
	return TeamSummary{Team: team, MyRole: RoleOwner}, nil
}

func (s *Service) ListMyTeams(ctx context.Context, actor reqctx.Identity) ([]TeamSummary, error) {
	summaries, err := s.repo.ListMyTeams(ctx, s.repo.pool, actor.UserID)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return summaries, nil
}

func (s *Service) GetTeam(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID) (TeamSummary, error) {
	m, err := s.member(ctx, actor, teamID)
	if err != nil {
		return TeamSummary{}, err
	}
	team, err := s.repo.TeamByID(ctx, s.repo.pool, teamID)
	if err != nil {
		return TeamSummary{}, httpx.Internal(err)
	}
	return TeamSummary{Team: team, MyRole: m.Role}, nil
}

func (s *Service) Permissions(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID) (Permissions, error) {
	m, err := s.member(ctx, actor, teamID)
	if err != nil {
		return Permissions{}, err
	}
	return permissionsFor(teamID, m.Role), nil
}

func (s *Service) ListMembers(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID) ([]Member, error) {
	if _, err := s.member(ctx, actor, teamID); err != nil {
		return nil, err
	}
	members, err := s.repo.ListMembers(ctx, s.repo.pool, teamID)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return members, nil
}

// Invite offers a role to an existing account. Only owner/admin may invite,
// only admin/member roles may be offered, and at most one pending invitation
// per team and user can exist (enforced by a partial unique index).
func (s *Service) Invite(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID, username, role string, expiresInHours int) (Invitation, error) {
	fields := make(map[string]string)
	if strings.TrimSpace(username) == "" {
		fields["username"] = "is required"
	}
	if !validInvitationRole(role) {
		fields["role"] = "must be admin or member"
	}
	if expiresInHours == 0 {
		expiresInHours = DefaultInvitationTTLHours
	}
	if expiresInHours < MinInvitationTTLHours || expiresInHours > MaxInvitationTTLHours {
		fields["expires_in_hours"] = fmt.Sprintf("must be between %d and %d", MinInvitationTTLHours, MaxInvitationTTLHours)
	}
	if len(fields) > 0 {
		return Invitation{}, httpx.ValidationFailed(fields)
	}

	account, err := s.accounts.AccountByUsername(ctx, username)
	if errors.Is(err, auth.ErrNotFound) {
		return Invitation{}, httpx.NotFound("the requested user does not exist")
	}
	if err != nil {
		return Invitation{}, httpx.Internal(err)
	}

	inv := Invitation{
		ID:              uuid.New(),
		TeamID:          teamID,
		TeamName:        "",
		InvitedUserID:   account.ID,
		InvitedUsername: account.Username,
		InvitedBy:       actor.UserID,
		Role:            role,
		Status:          InvitationPending,
		ExpiresAt:       time.Now().UTC().Add(time.Duration(expiresInHours) * time.Hour),
	}
	err = s.mutate(ctx, actor, teamID, func(tx pgx.Tx, actorMember Member) error {
		if err := s.requireManager(actor, teamID, actorMember); err != nil {
			return err
		}
		if _, err := s.repo.Membership(ctx, tx, teamID, account.ID); err == nil {
			return httpx.Conflict("the user is already an active member of this team")
		} else if !errors.Is(err, ErrNotFound) {
			return httpx.Internal(err)
		}
		if err := s.repo.InsertInvitation(ctx, tx, inv); err != nil {
			if errors.Is(err, ErrDuplicateInvitation) {
				return httpx.Conflict("a pending invitation already exists for this user")
			}
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "team.invitation_created", "team_invitation", &inv.ID, &teamID, map[string]any{
			"username": account.Username, "role": role, "expires_at": inv.ExpiresAt,
		}))
	})
	if err != nil {
		return Invitation{}, err
	}
	return inv, nil
}

func (s *Service) ListTeamInvitations(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID) ([]Invitation, error) {
	m, err := s.member(ctx, actor, teamID)
	if err != nil {
		return nil, err
	}
	if err := s.requireManager(actor, teamID, m); err != nil {
		return nil, s.unwrapRefusal(ctx, actor, teamID, err)
	}
	invitations, err := s.repo.ListTeamInvitations(ctx, s.repo.pool, teamID)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return invitations, nil
}

// CancelInvitation withdraws a pending invitation. The invitation must belong
// to the team in the request path; cross-team references resolve to 404.
func (s *Service) CancelInvitation(ctx context.Context, actor reqctx.Identity, teamID, invitationID uuid.UUID) error {
	return s.mutate(ctx, actor, teamID, func(tx pgx.Tx, actorMember Member) error {
		if err := s.requireManager(actor, teamID, actorMember); err != nil {
			return err
		}
		inv, err := s.repo.LockInvitation(ctx, tx, invitationID)
		if errors.Is(err, ErrNotFound) || (err == nil && inv.TeamID != teamID) {
			return httpx.NotFound("the requested invitation does not exist")
		}
		if err != nil {
			return httpx.Internal(err)
		}
		if inv.Status != InvitationPending {
			return httpx.Conflict("the invitation is no longer pending")
		}
		if err := s.repo.SetInvitationStatus(ctx, tx, invitationID, InvitationCancelled); err != nil {
			return httpx.Internal(err)
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "team.invitation_cancelled", "team_invitation", &invitationID, &teamID, nil))
	})
}

// ChangeMemberRole applies the role matrix: owners may grant any role;
// admins may only move between admin and member and never touch owners;
// members have no role authority. The last-owner guard lives in the database.
func (s *Service) ChangeMemberRole(ctx context.Context, actor reqctx.Identity, teamID, userID uuid.UUID, role string) error {
	if !validTeamRole(role) {
		return httpx.ValidationFailed(map[string]string{"role": "must be owner, admin or member"})
	}
	return s.mutate(ctx, actor, teamID, func(tx pgx.Tx, actorMember Member) error {
		if actorMember.Role == RoleMember {
			return &refusal{reason: "insufficient_role", err: httpx.Forbidden("your team role does not allow this operation")}
		}
		target, err := s.repo.Membership(ctx, tx, teamID, userID)
		if errors.Is(err, ErrNotFound) {
			return httpx.NotFound("the requested member does not exist")
		}
		if err != nil {
			return httpx.Internal(err)
		}
		if actorMember.Role == RoleAdmin {
			if role == RoleOwner {
				return &refusal{reason: "admin_cannot_grant_owner", err: httpx.Forbidden("team administrators cannot grant the owner role")}
			}
			if target.Role == RoleOwner {
				return &refusal{reason: "admin_cannot_change_owner", err: httpx.Forbidden("team administrators cannot change an owner's role")}
			}
		}
		if target.Role == role {
			return httpx.Conflict("the member already holds this role")
		}
		changed, err := s.repo.UpdateMemberRole(ctx, tx, teamID, userID, role)
		if errors.Is(err, ErrProjectLead) {
			return httpx.Conflict("hand off all projects before leaving the team")
		}
		if errors.Is(err, ErrLastOwner) {
			return httpx.Conflict("the team must retain at least one active owner")
		}
		if err != nil {
			return httpx.Internal(err)
		}
		if !changed {
			return httpx.NotFound("the requested member does not exist")
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "team.member_role_changed", "user", &userID, &teamID, map[string]any{
			"username": target.Username, "old_role": target.Role, "new_role": role,
		}))
	})
}

// RemoveMember removes another member (owner/admin, admins never owners) or
// lets any member leave. Removal takes effect immediately because membership
// is re-queried on every request.
func (s *Service) RemoveMember(ctx context.Context, actor reqctx.Identity, teamID, userID uuid.UUID) error {
	selfLeave := userID == actor.UserID
	return s.mutate(ctx, actor, teamID, func(tx pgx.Tx, actorMember Member) error {
		if !selfLeave && actorMember.Role == RoleMember {
			return &refusal{reason: "insufficient_role", err: httpx.Forbidden("your team role does not allow this operation")}
		}
		target, err := s.repo.Membership(ctx, tx, teamID, userID)
		if errors.Is(err, ErrNotFound) {
			return httpx.NotFound("the requested member does not exist")
		}
		if err != nil {
			return httpx.Internal(err)
		}
		if !selfLeave && actorMember.Role == RoleAdmin && target.Role == RoleOwner {
			return &refusal{reason: "admin_cannot_remove_owner", err: httpx.Forbidden("team administrators cannot remove an owner")}
		}
		removed, err := s.repo.RemoveMember(ctx, tx, teamID, userID)
		if errors.Is(err, ErrProjectLead) {
			return httpx.Conflict("hand off all projects before leaving the team")
		}
		if errors.Is(err, ErrLastOwner) {
			return httpx.Conflict("the team must retain at least one active owner")
		}
		if err != nil {
			return httpx.Internal(err)
		}
		if !removed {
			return httpx.NotFound("the requested member does not exist")
		}
		return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "team.member_removed", "user", &userID, &teamID, map[string]any{
			"username": target.Username, "role": target.Role, "self_leave": selfLeave,
		}))
	})
}

func (s *Service) ListMyInvitations(ctx context.Context, actor reqctx.Identity) ([]Invitation, error) {
	invitations, err := s.repo.ListMyInvitations(ctx, s.repo.pool, actor.UserID)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return invitations, nil
}

// AcceptInvitation joins the invitee to the invitation's team. Only the
// invited account may respond; everyone else receives 404. Expiry and status
// are re-checked under the invitation lock.
func (s *Service) AcceptInvitation(ctx context.Context, actor reqctx.Identity, invitationID uuid.UUID) (Invitation, error) {
	inv, err := s.respondInvitation(ctx, actor, invitationID, InvitationAccepted)
	if err != nil {
		return Invitation{}, err
	}
	return inv, nil
}

func (s *Service) DeclineInvitation(ctx context.Context, actor reqctx.Identity, invitationID uuid.UUID) error {
	_, err := s.respondInvitation(ctx, actor, invitationID, InvitationDeclined)
	return err
}

func (s *Service) respondInvitation(ctx context.Context, actor reqctx.Identity, invitationID uuid.UUID, response string) (Invitation, error) {
	preview, err := s.repo.InvitationByID(ctx, s.repo.pool, invitationID)
	if errors.Is(err, ErrNotFound) {
		return Invitation{}, httpx.NotFound("the requested invitation does not exist")
	}
	if err != nil {
		return Invitation{}, httpx.Internal(err)
	}
	if preview.InvitedUserID != actor.UserID {
		return Invitation{}, httpx.NotFound("the requested invitation does not exist")
	}

	var locked Invitation
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.LockTeam(ctx, tx, preview.TeamID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return httpx.NotFound("the requested invitation does not exist")
			}
			return httpx.Internal(err)
		}
		locked, err = s.repo.LockInvitation(ctx, tx, invitationID)
		if errors.Is(err, ErrNotFound) {
			return httpx.NotFound("the requested invitation does not exist")
		}
		if err != nil {
			return httpx.Internal(err)
		}
		if locked.Status != InvitationPending {
			return httpx.Conflict("the invitation is no longer pending")
		}
		if !locked.ExpiresAt.After(time.Now()) {
			return httpx.Conflict("the invitation has expired")
		}
		if response == InvitationAccepted {
			if err := s.repo.ActivateMember(ctx, tx, locked.TeamID, actor.UserID, locked.Role); err != nil {
				return httpx.Internal(err)
			}
		}
		if err := s.repo.SetInvitationStatus(ctx, tx, invitationID, response); err != nil {
			return httpx.Internal(err)
		}
		action := "team.invitation_declined"
		if response == InvitationAccepted {
			action = "team.invitation_accepted"
		}
		if err := s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, action, "team_invitation", &invitationID, &locked.TeamID, nil)); err != nil {
			return err
		}
		if response == InvitationAccepted {
			return s.audit.Insert(ctx, tx, s.event(ctx, &actor.UserID, "team.member_added", "user", &actor.UserID, &locked.TeamID, map[string]any{
				"role": locked.Role, "invitation_id": invitationID,
			}))
		}
		return nil
	})
	if err != nil {
		return Invitation{}, err
	}
	return locked, nil
}

// ListAllTeams is the platform administration channel (ROOT or platform
// admin). It grants visibility only, never team business capabilities.
func (s *Service) ListAllTeams(ctx context.Context, actor reqctx.Identity) ([]Team, error) {
	if !actor.SystemRoot && !actor.PlatformAdmin {
		return nil, httpx.Forbidden("only platform administrators may list all teams")
	}
	teams, err := s.repo.ListAllTeams(ctx, s.repo.pool)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return teams, nil
}
