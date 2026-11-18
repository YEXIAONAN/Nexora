package contributions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	repo     *Repository
	projects *projects.Service
}

func NewService(r *Repository, p *projects.Service) *Service { return &Service{r, p} }
func authenticated(a reqctx.Identity) error {
	if a.UserID == uuid.Nil || a.SessionID == uuid.Nil {
		return httpx.Unauthorized("authentication required")
	}
	return nil
}
func (s *Service) within(ctx context.Context, a reqctx.Identity, team, pid uuid.UUID, write bool, fn func(pgx.Tx, projects.Scope, Eligibility) error) error {
	if e := authenticated(a); e != nil {
		return e
	}
	e := s.projects.Within(ctx, a, team, pid, write, false, func(tx pgx.Tx, sc projects.Scope) error {
		ok, e := s.repo.session(ctx, tx, a)
		if e != nil {
			return e
		}
		if !ok {
			return httpx.Unauthorized("the session is invalid or has expired")
		}
		ok, e = s.projects.Qualified(ctx, tx, team, pid, a.UserID)
		if e != nil {
			return e
		}
		if !ok {
			return projects.NotFound()
		}
		el, e := s.repo.eligibility(ctx, tx, team, pid, a.UserID)
		if e != nil {
			return e
		}
		return fn(tx, sc, el)
	})
	return safeError(e)
}
func requestContext(ctx context.Context) context.Context {
	if reqctx.RequestID(ctx) == "" {
		return reqctx.WithRequestID(ctx, uuid.NewString())
	}
	return ctx
}
func (s *Service) requireVisible(ctx context.Context, tx pgx.Tx, c Case, a reqctx.Identity) error {
	ok, e := s.repo.visible(ctx, tx, c, a.UserID)
	if e != nil {
		return e
	}
	if !ok {
		return projects.NotFound()
	}
	return nil
}
func (s *Service) requireEditor(ctx context.Context, tx pgx.Tx, c Case, a reqctx.Identity, el Eligibility, version uuid.UUID) error {
	if _, e := s.repo.version(ctx, tx, c, version); e != nil {
		return e
	}
	if !el.CanEditCases {
		return projects.Forbidden()
	}
	b, e := s.repo.beneficiary(ctx, tx, version, a.UserID)
	if e != nil {
		return e
	}
	if b {
		return projects.Forbidden()
	}
	return nil
}
func (s *Service) requireIndependent(ctx context.Context, tx pgx.Tx, c Case, a reqctx.Identity, el Eligibility, version uuid.UUID) error {
	if _, e := s.repo.version(ctx, tx, c, version); e != nil {
		return e
	}
	if !el.ExplicitAppraiser {
		return projects.Forbidden()
	}
	ok, e := s.repo.independent(ctx, tx, version, a.UserID)
	if e != nil {
		return e
	}
	if !ok {
		return projects.Forbidden()
	}
	return nil
}
func (s *Service) replay(ctx context.Context, tx pgx.Tx, a reqctx.Identity, team, pid, key uuid.UUID, hash string) (CommandResult, bool, error) {
	prior, out, e := s.repo.priorCommand(ctx, tx, team, pid, a.UserID, key)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	if prior != hash {
		return out, false, httpx.Conflict("the idempotency key was used for a different request")
	}
	return out, true, nil
}
func (s *Service) completed(ctx context.Context, tx pgx.Tx, a reqctx.Identity, team, pid uuid.UUID, key uuid.UUID, hash, reason string, out CommandResult) error {
	return s.projects.Event(ctx, tx, a, team, pid, "contribution.command_completed", "project", map[string]any{"client_request_id": key, "input_digest": hash, "reason": reason, "result": out})
}

// Permission is checked before replay too: a revoked caller cannot use a prior
// idempotency key to regain access to historical business results.
func (s *Service) caseCommand(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID, kind string, cmd Command, payload any, authorize func(pgx.Tx, Case, Eligibility) error, mutate func(pgx.Tx, Case, string) (uuid.UUID, error)) (out CommandResult, err error) {
	ctx = requestContext(ctx)
	hash, e := digestOf(struct {
		Kind                string
		Team, Project, Case uuid.UUID
		Input               any
	}{kind, team, pid, cid, payload})
	if e != nil {
		return out, invalid("request", "cannot be encoded")
	}
	err = s.within(ctx, a, team, pid, true, func(tx pgx.Tx, sc projects.Scope, el Eligibility) error {
		c, e := s.repo.caseByID(ctx, tx, team, pid, cid, true)
		if e != nil {
			return e
		}
		if kind != "dispute" {
			if e = s.requireVisible(ctx, tx, c, a); e != nil {
				return e
			}
		}
		if e = authorize(tx, c, el); e != nil {
			return e
		}
		var replay bool
		if out, replay, e = s.replay(ctx, tx, a, team, pid, cmd.ClientRequestID, hash); e != nil || replay {
			return e
		}
		if e = projects.CheckVersion(cmd.ExpectedVersion, c.Revision); e != nil {
			return e
		}
		id, e := mutate(tx, c, reqctx.RequestID(ctx))
		if e != nil {
			return e
		}
		after, e := s.repo.caseByID(ctx, tx, team, pid, cid, false)
		if e != nil {
			return e
		}
		out = CommandResult{id, &cid, after.Revision, cmd.ClientRequestID}
		return s.completed(ctx, tx, a, team, pid, cmd.ClientRequestID, hash, cmd.Reason, out)
	})
	return
}
func (s *Service) Appraisers(ctx context.Context, a reqctx.Identity, team, pid uuid.UUID) (out []Appraiser, err error) {
	err = s.within(ctx, a, team, pid, false, func(tx pgx.Tx, _ projects.Scope, _ Eligibility) error {
		var e error
		out, e = s.repo.appraisers(ctx, tx, team, pid)
		return e
	})
	return
}
func (s *Service) Eligibility(ctx context.Context, a reqctx.Identity, team, pid uuid.UUID) (out Eligibility, err error) {
	err = s.within(ctx, a, team, pid, false, func(_ pgx.Tx, _ projects.Scope, el Eligibility) error { out = el; return nil })
	return
}
func (s *Service) SetAppraiser(ctx context.Context, a reqctx.Identity, team, pid, user uuid.UUID, in Command, revoke bool) (out CommandResult, err error) {
	if e := normalizeCommand(&in, !revoke); e != nil {
		return out, e
	}
	if !bounded(in.Reason, 1000) {
		return out, invalid("reason", "must contain 1–1000 characters")
	}
	if user == uuid.Nil {
		return out, invalid("user_id", "is required")
	}
	ctx = requestContext(ctx)
	hash, _ := digestOf(struct {
		User   uuid.UUID
		Revoke bool
		Input  Command
	}{user, revoke, in})
	err = s.within(ctx, a, team, pid, true, func(tx pgx.Tx, sc projects.Scope, el Eligibility) error {
		allowed := sc.Lead || (user == sc.Project.LeadID && (sc.Member.Role == teams.RoleOwner || sc.Member.Role == teams.RoleAdmin || el.ExplicitAppraiser)) || (revoke && user == a.UserID)
		if !allowed || (!revoke && user == a.UserID) {
			return projects.Forbidden()
		}
		var replay bool
		var e error
		if out, replay, e = s.replay(ctx, tx, a, team, pid, in.ClientRequestID, hash); e != nil || replay {
			return e
		}
		if !revoke {
			ok, e := s.projects.Qualified(ctx, tx, team, pid, user)
			if e != nil {
				return e
			}
			if !ok {
				return projects.NotFound()
			}
		}
		if e = s.repo.grant(ctx, tx, team, pid, user, a.UserID, in, revoke, reqctx.RequestID(ctx)); e != nil {
			return e
		}
		var version int
		if e = tx.QueryRow(ctx, `SELECT version FROM contribution_appraisers WHERE team_id=$1 AND project_id=$2 AND user_id=$3`, team, pid, user).Scan(&version); e != nil {
			return e
		}
		out = CommandResult{user, nil, version, in.ClientRequestID}
		return s.completed(ctx, tx, a, team, pid, in.ClientRequestID, hash, in.Reason, out)
	})
	return
}
func (s *Service) Create(ctx context.Context, a reqctx.Identity, team, pid uuid.UUID, in CreateInput) (out CommandResult, err error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if in.TaskID == uuid.Nil || in.ReviewID == uuid.Nil || in.ClientRequestID == uuid.Nil {
		return out, invalid("request", "task_id, review_id and client_request_id are required")
	}
	if in.Reason == "" || !bounded(in.Reason, 4000) {
		return out, invalid("reason", "must contain 1–4000 characters")
	}
	ctx = requestContext(ctx)
	hash, _ := digestOf(struct {
		Kind  string
		Input CreateInput
	}{"create", in})
	err = s.within(ctx, a, team, pid, true, func(tx pgx.Tx, _ projects.Scope, el Eligibility) error {
		if !el.CanEditCases {
			return projects.Forbidden()
		}
		var replay bool
		var e error
		if out, replay, e = s.replay(ctx, tx, a, team, pid, in.ClientRequestID, hash); e != nil || replay {
			return e
		}
		ok, e := s.repo.accepted(ctx, tx, team, pid, in.TaskID, in.ReviewID)
		if e != nil {
			return e
		}
		if !ok {
			return projects.NotFound()
		}
		id, e := s.repo.create(ctx, tx, team, pid, in, a.UserID, reqctx.RequestID(ctx))
		if e != nil {
			return e
		}
		c, e := s.repo.caseByID(ctx, tx, team, pid, id, false)
		if e != nil {
			return e
		}
		out = CommandResult{id, &id, c.Revision, in.ClientRequestID}
		return s.completed(ctx, tx, a, team, pid, in.ClientRequestID, hash, in.Reason, out)
	})
	return
}
func (s *Service) List(ctx context.Context, a reqctx.Identity, team, pid uuid.UUID, pending bool) (out []Case, err error) {
	err = s.within(ctx, a, team, pid, false, func(tx pgx.Tx, _ projects.Scope, _ Eligibility) error {
		var e error
		out, e = s.repo.cases(ctx, tx, team, pid, a.UserID, pending)
		return e
	})
	return
}
func (s *Service) readCase(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID, fn func(pgx.Tx, Case, projects.Scope, Eligibility) error) error {
	return s.within(ctx, a, team, pid, false, func(tx pgx.Tx, sc projects.Scope, el Eligibility) error {
		c, e := s.repo.caseByID(ctx, tx, team, pid, cid, false)
		if e != nil {
			return e
		}
		if e = s.requireVisible(ctx, tx, c, a); e != nil {
			return e
		}
		return fn(tx, c, sc, el)
	})
}
func (s *Service) permissions(ctx context.Context, tx pgx.Tx, c Case, v Version, a reqctx.Identity, el Eligibility, active bool) (p Permissions, e error) {
	if !active {
		return p, nil
	}
	b, e := s.repo.beneficiary(ctx, tx, v.ID, a.UserID)
	if e != nil {
		return p, e
	}
	ind, e := s.repo.independent(ctx, tx, v.ID, a.UserID)
	if e != nil {
		return p, e
	}
	p.CanEdit = el.CanEditCases && !b && v.Status == "draft"
	p.CanSubmit = p.CanEdit && v.ReadyToSubmit
	p.CanWithdraw = el.CanEditCases && !b && v.Status == "pending_review" && v.ProposedBy == a.UserID
	p.CanConfirm = ind && v.Status == "pending_review" && c.CaseWorkflowStatus != "disputed"
	p.CanRevise = el.CanEditCases && !b && v.Status != "draft" && v.Status != "pending_review" && c.CaseWorkflowStatus != "disputed"
	p.CanDispute = c.EffectiveConfirmedVersionID != nil
	p.CanNotify = el.CanEditCases && c.EffectiveConfirmedVersionID != nil
	return p, nil
}
func (s *Service) Get(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID) (out CaseDetail, err error) {
	err = s.readCase(ctx, a, team, pid, cid, func(tx pgx.Tx, c Case, sc projects.Scope, el Eligibility) error {
		out.Case = c
		var e error
		if out.Latest, e = s.repo.version(ctx, tx, c, c.LatestVersionID); e != nil {
			return e
		}
		if c.EffectiveConfirmedVersionID != nil {
			v, e := s.repo.version(ctx, tx, c, *c.EffectiveConfirmedVersionID)
			if e != nil {
				return e
			}
			out.Effective = &v
		}
		out.Permissions, e = s.permissions(ctx, tx, c, out.Latest, a, el, sc.Project.Status == "active")
		return e
	})
	return
}
func (s *Service) Versions(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID) (out []Version, err error) {
	err = s.readCase(ctx, a, team, pid, cid, func(tx pgx.Tx, c Case, _ projects.Scope, _ Eligibility) error {
		var e error
		out, e = s.repo.versions(ctx, tx, c)
		return e
	})
	return
}
func (s *Service) History(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID) (out History, err error) {
	err = s.readCase(ctx, a, team, pid, cid, func(tx pgx.Tx, c Case, _ projects.Scope, el Eligibility) error {
		var e error
		out, e = s.repo.history(ctx, tx, c)
		if e != nil {
			return e
		}
		return s.rulingPermissions(ctx, tx, c, a, el, out.Disputes)
	})
	return
}
func (s *Service) Evidence(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID) (out AcceptanceEvidence, err error) {
	err = s.readCase(ctx, a, team, pid, cid, func(tx pgx.Tx, c Case, _ projects.Scope, _ Eligibility) error {
		var e error
		out, e = s.repo.evidence(ctx, tx, c)
		return e
	})
	return
}
func (s *Service) draftEditor(ctx context.Context, tx pgx.Tx, c Case, a reqctx.Identity, el Eligibility, id uuid.UUID) error {
	return s.requireEditor(ctx, tx, c, a, el, id)
}
func latest(c Case, id uuid.UUID) error {
	if c.LatestVersionID != id {
		return httpx.Conflict("only the latest work version can be changed")
	}
	return nil
}
func (s *Service) SaveDraft(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID, in DraftInput) (CommandResult, error) {
	if e := normalizeVersion(&in.VersionCommand); e != nil {
		return CommandResult{}, e
	}
	if !bounded(in.Summary, 8000) {
		return CommandResult{}, invalid("summary", "must be at most 8000 characters")
	}
	if in.EvidenceSnapshot == nil {
		return CommandResult{}, invalid("evidence_snapshot", "must be an object")
	}
	b, e := json.Marshal(in.EvidenceSnapshot)
	if e != nil || len(b) > 32768 || !validJSONValue(in.EvidenceSnapshot) {
		return CommandResult{}, invalid("evidence_snapshot", "must be an object within 32 KiB")
	}
	return s.caseCommand(ctx, a, team, pid, cid, "draft", in.Command, in, func(tx pgx.Tx, c Case, el Eligibility) error {
		return s.draftEditor(ctx, tx, c, a, el, in.CaseVersionID)
	}, func(tx pgx.Tx, c Case, request string) (uuid.UUID, error) {
		if e := latest(c, in.CaseVersionID); e != nil {
			return uuid.Nil, e
		}
		v, e := s.repo.version(ctx, tx, c, in.CaseVersionID)
		if e != nil {
			return uuid.Nil, e
		}
		if v.Status != "draft" {
			return uuid.Nil, httpx.Conflict("only drafts can be edited")
		}
		return in.CaseVersionID, s.repo.saveDraft(ctx, tx, c, a.UserID, in, request)
	})
}
func (s *Service) SetShare(ctx context.Context, a reqctx.Identity, team, pid, cid, user uuid.UUID, in ShareInput, remove bool) (CommandResult, error) {
	if e := normalizeShare(&in, remove); e != nil {
		return CommandResult{}, e
	}
	if user == uuid.Nil {
		return CommandResult{}, invalid("user_id", "is required")
	}
	return s.caseCommand(ctx, a, team, pid, cid, "share", in.Command, struct {
		User   uuid.UUID
		Remove bool
		Input  ShareInput
	}{user, remove, in}, func(tx pgx.Tx, c Case, el Eligibility) error {
		if user == a.UserID {
			return projects.Forbidden()
		}
		return s.draftEditor(ctx, tx, c, a, el, in.CaseVersionID)
	}, func(tx pgx.Tx, c Case, request string) (uuid.UUID, error) {
		if e := latest(c, in.CaseVersionID); e != nil {
			return uuid.Nil, e
		}
		ok, e := s.repo.memberExists(ctx, tx, team, pid, user)
		if e != nil {
			return uuid.Nil, e
		}
		if !ok {
			return uuid.Nil, projects.NotFound()
		}
		return in.CaseVersionID, s.repo.setShare(ctx, tx, c, user, a.UserID, in, request)
	})
}
func (s *Service) VersionAction(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID, action string, in VersionCommand) (CommandResult, error) {
	if e := normalizeVersion(&in); e != nil {
		return CommandResult{}, e
	}
	return s.caseCommand(ctx, a, team, pid, cid, action, in.Command, in, func(tx pgx.Tx, c Case, el Eligibility) error {
		if action == "confirm" {
			return s.requireIndependent(ctx, tx, c, a, el, in.CaseVersionID)
		}
		if e := s.requireEditor(ctx, tx, c, a, el, in.CaseVersionID); e != nil {
			return e
		}
		if action == "withdraw" {
			v, e := s.repo.version(ctx, tx, c, in.CaseVersionID)
			if e != nil {
				return e
			}
			if v.ProposedBy != a.UserID {
				return projects.Forbidden()
			}
		}
		return nil
	}, func(tx pgx.Tx, c Case, request string) (uuid.UUID, error) {
		if e := latest(c, in.CaseVersionID); e != nil {
			return uuid.Nil, e
		}
		switch action {
		case "submit":
			return in.CaseVersionID, s.repo.transition(ctx, tx, c, a.UserID, in, "pending_review", request)
		case "withdraw":
			return in.CaseVersionID, s.repo.transition(ctx, tx, c, a.UserID, in, "draft", request)
		case "abandon":
			return in.CaseVersionID, s.repo.transition(ctx, tx, c, a.UserID, in, "abandoned", request)
		case "confirm":
			return s.repo.confirm(ctx, tx, c, a.UserID, in, request)
		case "revisions":
			return s.repo.revise(ctx, tx, c, a.UserID, in, request)
		default:
			return uuid.Nil, httpx.BadRequest("unknown contribution action")
		}
	})
}
func (s *Service) Disputes(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID) (out []Dispute, err error) {
	err = s.readCase(ctx, a, team, pid, cid, func(tx pgx.Tx, c Case, _ projects.Scope, el Eligibility) error {
		var e error
		out, e = s.repo.disputes(ctx, tx, c)
		if e != nil {
			return e
		}
		return s.rulingPermissions(ctx, tx, c, a, el, out)
	})
	return
}
func (s *Service) rulingPermissions(ctx context.Context, tx pgx.Tx, c Case, a reqctx.Identity, el Eligibility, out []Dispute) error {
	for i := range out {
		d := &out[i]
		if d.Resolution != nil || !el.ExplicitAppraiser || d.RaisedBy == a.UserID || (d.OnBehalfOf != nil && *d.OnBehalfOf == a.UserID) {
			continue
		}
		v, e := s.repo.version(ctx, tx, c, d.CaseVersionID)
		if e != nil {
			return e
		}
		ok, e := s.repo.independent(ctx, tx, v.ID, a.UserID)
		if e != nil {
			return e
		}
		d.CanResolve = ok && v.Confirmation != nil && v.Confirmation.ConfirmedBy != a.UserID
	}
	return nil
}
func (s *Service) RaiseDispute(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID, in DisputeInput) (CommandResult, error) {
	if e := normalizeVersion(&in.VersionCommand); e != nil {
		return CommandResult{}, e
	}
	if in.ConfirmationID == uuid.Nil {
		return CommandResult{}, invalid("confirmation_id", "is required")
	}
	if (in.OnBehalfOf == nil) != (in.IdentityRequestID == nil) {
		return CommandResult{}, invalid("identity_request_id", "a proxy dispute requires the subject and a verified request")
	}
	return s.caseCommand(ctx, a, team, pid, cid, "dispute", in.Command, in, func(_ pgx.Tx, _ Case, el Eligibility) error {
		if in.OnBehalfOf != nil && (!el.CanEditCases || *in.OnBehalfOf == a.UserID) {
			return projects.Forbidden()
		}
		return nil
	}, func(tx pgx.Tx, c Case, request string) (uuid.UUID, error) {
		if c.EffectiveConfirmedVersionID == nil || *c.EffectiveConfirmedVersionID != in.CaseVersionID {
			return uuid.Nil, httpx.Conflict("dispute must identify the current effective confirmation")
		}
		f, e := s.repo.confirmation(ctx, tx, c, in.CaseVersionID)
		if e != nil {
			return uuid.Nil, e
		}
		if f == nil || f.ID != in.ConfirmationID {
			return uuid.Nil, projects.NotFound()
		}
		if in.OnBehalfOf != nil {
			var ok bool
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contribution_attestations WHERE id=$1 AND team_id=$2 AND project_id=$3 AND task_id=$4 AND case_id=$5 AND case_version_id=$6 AND confirmation_id=$7 AND purpose='dispute_request' AND subject_id=$8 AND content_digest=$9)`, *in.IdentityRequestID, team, pid, c.TaskID, c.ID, in.CaseVersionID, in.ConfirmationID, *in.OnBehalfOf, digestString(in.Reason)).Scan(&ok)
			if e != nil {
				return uuid.Nil, e
			}
			if !ok {
				return uuid.Nil, projects.NotFound()
			}
		}
		return s.repo.dispute(ctx, tx, c, a.UserID, in, request)
	})
}
func (s *Service) Resolve(ctx context.Context, a reqctx.Identity, team, pid, cid, dispute uuid.UUID, in ResolutionInput) (CommandResult, error) {
	if e := normalizeCommand(&in.Command, false); e != nil {
		return CommandResult{}, e
	}
	if in.Decision != "uphold" && in.Decision != "overturn" {
		return CommandResult{}, invalid("decision", "must be uphold or overturn")
	}
	return s.caseCommand(ctx, a, team, pid, cid, "resolve", in.Command, struct {
		Dispute uuid.UUID
		Input   ResolutionInput
	}{dispute, in}, func(tx pgx.Tx, c Case, el Eligibility) error {
		ds, e := s.repo.disputes(ctx, tx, c)
		if e != nil {
			return e
		}
		for _, d := range ds {
			if d.ID != dispute {
				continue
			}
			if e = s.requireIndependent(ctx, tx, c, a, el, d.CaseVersionID); e != nil {
				return e
			}
			f, e := s.repo.confirmation(ctx, tx, c, d.CaseVersionID)
			if e != nil {
				return e
			}
			if f == nil {
				return projects.NotFound()
			}
			if f.ConfirmedBy == a.UserID || d.RaisedBy == a.UserID || (d.OnBehalfOf != nil && *d.OnBehalfOf == a.UserID) {
				return projects.Forbidden()
			}
			return nil
		}
		return projects.NotFound()
	}, func(tx pgx.Tx, c Case, request string) (uuid.UUID, error) {
		return s.repo.resolve(ctx, tx, c, dispute, a.UserID, in, request)
	})
}
func (s *Service) RecordConflict(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID, in ConflictInput) (CommandResult, error) {
	if e := normalizeCommand(&in.Command, false); e != nil {
		return CommandResult{}, e
	}
	in.Kind = strings.TrimSpace(in.Kind)
	in.Details = strings.TrimSpace(in.Details)
	if in.UserID == uuid.Nil || in.Kind == "" || !bounded(in.Kind, 100) || in.Details == "" || !bounded(in.Details, 4000) {
		return CommandResult{}, invalid("conflict", "provide user_id, kind and details within the length limits")
	}
	return s.caseCommand(ctx, a, team, pid, cid, "conflict", in.Command, in, func(tx pgx.Tx, c Case, el Eligibility) error {
		if in.UserID != a.UserID && !el.CanEditCases {
			return projects.Forbidden()
		}
		return s.requireVisible(ctx, tx, c, a)
	}, func(tx pgx.Tx, c Case, request string) (uuid.UUID, error) {
		ok, e := s.repo.memberExists(ctx, tx, team, pid, in.UserID)
		if e != nil {
			return uuid.Nil, e
		}
		if !ok {
			return uuid.Nil, projects.NotFound()
		}
		return s.repo.conflict(ctx, tx, c, a.UserID, in, request)
	})
}
