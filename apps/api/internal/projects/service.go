package projects

import (
	"context"
	"errors"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct {
	repo   *Repository
	teams  *teams.Repository
	audit  *audit.Repository
	logger *slog.Logger
}

func NewService(repo *Repository, t *teams.Repository, a *audit.Repository, l *slog.Logger) *Service {
	return &Service{repo, t, a, l}
}

type Scope struct {
	Project Project
	Member  teams.Member
	Lead    bool
}
type denial struct{ err error }

func (d *denial) Error() string { return d.err.Error() }
func (d *denial) Unwrap() error { return d.err }
func Forbidden() error {
	return &denial{httpx.Forbidden("your project role does not allow this operation")}
}
func NotFound() error { return &denial{httpx.NotFound("the requested resource does not exist")} }
func CheckVersion(expected, actual int) error {
	if expected != actual {
		return httpx.Conflict("the resource changed; reload before retrying")
	}
	return nil
}
func ValidateReason(reason string) error {
	if n := utf8.RuneCountInString(strings.TrimSpace(reason)); n < 1 || n > 1000 {
		return httpx.ValidationFailed(map[string]string{"reason": "must be between 1 and 1000 characters"})
	}
	return nil
}
func (s *Service) Event(ctx context.Context, tx pgx.Tx, actor reqctx.Identity, teamID, targetID uuid.UUID, action, targetType string, details map[string]any) error {
	return s.audit.Insert(ctx, tx, audit.Event{ActorID: &actor.UserID, Action: action, TeamID: &teamID, TargetID: &targetID, TargetType: targetType, RequestID: reqctx.RequestID(ctx), Details: details})
}

// withTeam takes the same first lock as Phase 02 membership mutations. Even
// reads use it so multi-query resource responses are consistent with removal.
func (s *Service) withTeam(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID, fn func(pgx.Tx, teams.Member) error) error {
	tx, err := s.repo.pool.Begin(ctx)
	if err != nil {
		return httpx.Internal(err)
	}
	defer tx.Rollback(ctx) // rollback after commit is harmless
	err = func() error {
		if e := s.teams.LockTeam(ctx, tx, teamID); e != nil {
			if errors.Is(e, teams.ErrNotFound) {
				return NotFound()
			}
			return e
		}
		m, e := s.teams.Membership(ctx, tx, teamID, actor.UserID)
		if e != nil {
			if errors.Is(e, teams.ErrNotFound) {
				return NotFound()
			}
			return e
		}
		return fn(tx, m)
	}()
	if err == nil {
		err = tx.Commit(ctx)
	} else {
		_ = tx.Rollback(ctx)
	}
	var d *denial
	if errors.As(err, &d) {
		ev := audit.Event{ActorID: &actor.UserID, Action: "authz.denied", TargetType: "project", TeamID: &teamID, RequestID: reqctx.RequestID(ctx), Details: map[string]any{"reason": "project_access_denied"}}
		if e := s.audit.InsertStandalone(ctx, ev); e != nil {
			s.logger.Warn("project refusal audit failed")
		}
		return d.err
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.ConstraintName == "project_member_has_tasks" {
		return httpx.Conflict("reassign active tasks before removing this project member")
	}
	if err != nil && !httpx.IsAPIError(err) {
		return httpx.Internal(err)
	}
	return err
}

// Within authorizes a project scope before handing the same transaction to a
// module. Callbacks own their domain writes, history and audit atomically.
func (s *Service) Within(ctx context.Context, actor reqctx.Identity, teamID, projectID uuid.UUID, write, leadOnly bool, fn func(pgx.Tx, Scope) error) error {
	return s.withTeam(ctx, actor, teamID, func(tx pgx.Tx, m teams.Member) error {
		p, err := s.repo.get(ctx, tx, teamID, projectID)
		if err != nil {
			if missing(err) {
				return NotFound()
			}
			return err
		}
		scope := Scope{Project: p, Member: m, Lead: p.LeadID == actor.UserID}
		if leadOnly && !scope.Lead {
			return Forbidden()
		}
		if write && p.Status != "active" {
			return httpx.Conflict("archived projects are read-only")
		}
		return fn(tx, scope)
	})
}
func (s *Service) Qualified(ctx context.Context, tx pgx.Tx, teamID, projectID, userID uuid.UUID) (bool, error) {
	return s.repo.Qualified(ctx, tx, teamID, projectID, userID)
}
func capabilities(p Project, m teams.Member, actor reqctx.Identity) Project {
	p.CanManage = p.Status == "active" && p.LeadID == actor.UserID
	p.CanHandoff = p.Status == "active" && (p.LeadID == actor.UserID || m.Role == teams.RoleOwner || m.Role == teams.RoleAdmin)
	return p
}
func validateInput(in *ProjectInput) error {
	in.Name = strings.TrimSpace(in.Name)
	f := map[string]string{}
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 120 {
		f["name"] = "must be between 1 and 120 characters"
	}
	if utf8.RuneCountInString(in.Description) > 4000 {
		f["description"] = "must be at most 4000 characters"
	}
	if in.PlannedStartAt != nil && in.PlannedEndAt != nil && in.PlannedStartAt.After(*in.PlannedEndAt) {
		f["planned_end_at"] = "must not precede planned_start_at"
	}
	if len(f) > 0 {
		return httpx.ValidationFailed(f)
	}
	return nil
}
func (s *Service) Create(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID, in CreateInput) (p Project, err error) {
	if err = validateInput(&in.ProjectInput); err != nil {
		return
	}
	err = s.withTeam(ctx, actor, teamID, func(tx pgx.Tx, m teams.Member) error {
		if m.Role != teams.RoleOwner && m.Role != teams.RoleAdmin {
			return Forbidden()
		}
		if _, e := s.teams.Membership(ctx, tx, teamID, in.LeadID); e != nil {
			if errors.Is(e, teams.ErrNotFound) {
				return httpx.ValidationFailed(map[string]string{"lead_id": "requires active team membership"})
			}
			return e
		}
		p = Project{ID: uuid.New(), TeamID: teamID, Name: in.Name, Description: in.Description, LeadID: in.LeadID, PlannedStartAt: in.PlannedStartAt, PlannedEndAt: in.PlannedEndAt, CreatedBy: actor.UserID}
		if e := s.repo.insert(ctx, tx, &p); e != nil {
			return e
		}
		for _, id := range []uuid.UUID{actor.UserID, p.LeadID} {
			if e := s.repo.addMember(ctx, tx, teamID, p.ID, id); e != nil {
				return e
			}
		}
		p = capabilities(p, m, actor)
		return s.Event(ctx, tx, actor, teamID, p.ID, "project.created", "project", map[string]any{"name": p.Name, "lead_id": p.LeadID})
	})
	return
}
func (s *Service) List(ctx context.Context, actor reqctx.Identity, teamID uuid.UUID) (out []Project, err error) {
	err = s.withTeam(ctx, actor, teamID, func(tx pgx.Tx, m teams.Member) error {
		var e error
		out, e = s.repo.list(ctx, tx, teamID)
		for i := range out {
			out[i] = capabilities(out[i], m, actor)
		}
		return e
	})
	return
}
func (s *Service) Get(ctx context.Context, actor reqctx.Identity, teamID, id uuid.UUID) (out Project, err error) {
	err = s.Within(ctx, actor, teamID, id, false, false, func(tx pgx.Tx, sc Scope) error { out = capabilities(sc.Project, sc.Member, actor); return nil })
	return
}
func (s *Service) Edit(ctx context.Context, actor reqctx.Identity, teamID, id uuid.UUID, in EditInput) (out Project, err error) {
	if err = validateInput(&in.ProjectInput); err != nil {
		return
	}
	err = s.Within(ctx, actor, teamID, id, true, true, func(tx pgx.Tx, sc Scope) error {
		if e := CheckVersion(in.ExpectedVersion, sc.Project.Version); e != nil {
			return e
		}
		out = sc.Project
		out.Name = in.Name
		out.Description = in.Description
		out.PlannedStartAt = in.PlannedStartAt
		out.PlannedEndAt = in.PlannedEndAt
		if e := s.repo.save(ctx, tx, &out); e != nil {
			return e
		}
		out = capabilities(out, sc.Member, actor)
		return s.Event(ctx, tx, actor, teamID, id, "project.edited", "project", map[string]any{"before": sc.Project, "after": out})
	})
	return
}
func (s *Service) Archive(ctx context.Context, actor reqctx.Identity, teamID, id uuid.UUID, version int) (out Project, err error) {
	err = s.Within(ctx, actor, teamID, id, true, true, func(tx pgx.Tx, sc Scope) error {
		if e := CheckVersion(version, sc.Project.Version); e != nil {
			return e
		}
		out = sc.Project
		now := time.Now().UTC()
		out.Status = "archived"
		out.ArchivedAt = &now
		if e := s.repo.save(ctx, tx, &out); e != nil {
			return e
		}
		out = capabilities(out, sc.Member, actor)
		return s.Event(ctx, tx, actor, teamID, id, "project.archived", "project", nil)
	})
	return
}
func (s *Service) Handoff(ctx context.Context, actor reqctx.Identity, teamID, id, newLead uuid.UUID, version int, reason string) (out Project, err error) {
	if err = ValidateReason(reason); err != nil {
		return
	}
	err = s.Within(ctx, actor, teamID, id, true, false, func(tx pgx.Tx, sc Scope) error {
		if !sc.Lead && sc.Member.Role != teams.RoleOwner && sc.Member.Role != teams.RoleAdmin {
			return Forbidden()
		}
		if e := CheckVersion(version, sc.Project.Version); e != nil {
			return e
		}
		if newLead == sc.Project.LeadID {
			return httpx.Conflict("this user is already the project lead")
		}
		if _, e := s.teams.Membership(ctx, tx, teamID, newLead); e != nil {
			if errors.Is(e, teams.ErrNotFound) {
				return httpx.ValidationFailed(map[string]string{"lead_id": "requires active team membership"})
			}
			return e
		}
		if e := s.repo.addMember(ctx, tx, teamID, id, newLead); e != nil {
			return e
		}
		out = sc.Project
		out.LeadID = newLead
		if e := s.repo.save(ctx, tx, &out); e != nil {
			return e
		}
		out = capabilities(out, sc.Member, actor)
		return s.Event(ctx, tx, actor, teamID, id, "project.lead_handed_off", "project", map[string]any{"old_lead_id": sc.Project.LeadID, "new_lead_id": newLead, "reason": strings.TrimSpace(reason)})
	})
	return
}
func (s *Service) Members(ctx context.Context, actor reqctx.Identity, teamID, id uuid.UUID) (out []Member, err error) {
	err = s.Within(ctx, actor, teamID, id, false, false, func(tx pgx.Tx, sc Scope) error { var e error; out, e = s.repo.members(ctx, tx, sc.Project); return e })
	return
}
func (s *Service) ChangeMember(ctx context.Context, actor reqctx.Identity, teamID, id, userID uuid.UUID, add bool) (out []Member, err error) {
	err = s.Within(ctx, actor, teamID, id, true, true, func(tx pgx.Tx, sc Scope) error {
		if add {
			if _, e := s.teams.Membership(ctx, tx, teamID, userID); e != nil {
				if errors.Is(e, teams.ErrNotFound) {
					return httpx.ValidationFailed(map[string]string{"user_id": "requires active team membership"})
				}
				return e
			}
			ok, e := s.Qualified(ctx, tx, teamID, id, userID)
			if e != nil {
				return e
			}
			if ok {
				return httpx.Conflict("the user is already a project member")
			}
			if e = s.repo.addMember(ctx, tx, teamID, id, userID); e != nil {
				return e
			}
		} else {
			if userID == sc.Project.LeadID {
				return httpx.Conflict("hand off the project lead before removal")
			}
			if e := s.repo.removeMember(ctx, tx, teamID, id, userID); e != nil {
				if missing(e) {
					return NotFound()
				}
				return e
			}
		}
		action := "project.member_removed"
		if add {
			action = "project.member_added"
		}
		if e := s.Event(ctx, tx, actor, teamID, id, action, "project", map[string]any{"user_id": userID}); e != nil {
			return e
		}
		var e error
		out, e = s.repo.members(ctx, tx, sc.Project)
		return e
	})
	return
}
func (s *Service) Milestones(ctx context.Context, actor reqctx.Identity, teamID, id uuid.UUID) (out []Milestone, err error) {
	err = s.Within(ctx, actor, teamID, id, false, false, func(tx pgx.Tx, sc Scope) error {
		var e error
		out, e = s.repo.milestones(ctx, tx, teamID, id)
		return e
	})
	return
}
func (s *Service) SaveMilestone(ctx context.Context, actor reqctx.Identity, teamID, id, milestoneID uuid.UUID, in MilestoneInput) (out Milestone, err error) {
	v := ProjectInput{Name: in.Name, Description: in.Description}
	if err = validateInput(&v); err != nil {
		return
	}
	err = s.Within(ctx, actor, teamID, id, true, true, func(tx pgx.Tx, sc Scope) error {
		create := milestoneID == uuid.Nil
		if create {
			out = Milestone{ID: uuid.New(), TeamID: teamID, ProjectID: id}
		} else {
			var e error
			out, e = s.repo.milestone(ctx, tx, teamID, id, milestoneID)
			if e != nil {
				if missing(e) {
					return NotFound()
				}
				return e
			}
			if e = CheckVersion(in.ExpectedVersion, out.Version); e != nil {
				return e
			}
		}
		out.Name = v.Name
		out.Description = v.Description
		out.PlannedAt = in.PlannedAt
		if e := s.repo.saveMilestone(ctx, tx, &out, create); e != nil {
			return e
		}
		action := "project.milestone_edited"
		if create {
			action = "project.milestone_created"
		}
		return s.Event(ctx, tx, actor, teamID, out.ID, action, "project_milestone", map[string]any{"project_id": id, "milestone": out})
	})
	return
}
