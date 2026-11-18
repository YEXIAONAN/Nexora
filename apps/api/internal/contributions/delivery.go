package contributions

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

func (s *Service) Notices(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID) (out []Notice, err error) {
	err = s.readCase(ctx, a, team, pid, cid, func(tx pgx.Tx, c Case, _ projects.Scope, _ Eligibility) error {
		var e error
		out, e = s.repo.notices(ctx, tx, team, pid, &c.ID, nil)
		return e
	})
	return
}
func (s *Service) PrepareNotice(ctx context.Context, a reqctx.Identity, team, pid, cid uuid.UUID, in NoticeInput) (CommandResult, error) {
	if e := normalizeVersion(&in.VersionCommand); e != nil {
		return CommandResult{}, e
	}
	if in.ConfirmationID == uuid.Nil || in.RecipientID == uuid.Nil {
		return CommandResult{}, invalid("notice", "confirmation_id and recipient_id are required")
	}
	return s.caseCommand(ctx, a, team, pid, cid, "notice", in.Command, in, func(_ pgx.Tx, _ Case, el Eligibility) error {
		if !el.CanEditCases {
			return projects.Forbidden()
		}
		return nil
	}, func(tx pgx.Tx, c Case, request string) (uuid.UUID, error) {
		if c.EffectiveConfirmedVersionID == nil || *c.EffectiveConfirmedVersionID != in.CaseVersionID {
			return uuid.Nil, httpx.Conflict("notice must identify the current effective confirmation")
		}
		f, e := s.repo.confirmation(ctx, tx, c, in.CaseVersionID)
		if e != nil {
			return uuid.Nil, e
		}
		if f == nil || f.ID != in.ConfirmationID {
			return uuid.Nil, projects.NotFound()
		}
		ok, e := s.repo.beneficiary(ctx, tx, in.CaseVersionID, in.RecipientID)
		if e != nil {
			return uuid.Nil, e
		}
		if !ok {
			return uuid.Nil, projects.NotFound()
		}
		return s.repo.notice(ctx, tx, c, a.UserID, in, request)
	})
}
func (s *Service) MyResults(ctx context.Context, a reqctx.Identity, team, pid uuid.UUID) (out []Result, err error) {
	out = []Result{}
	err = s.within(ctx, a, team, pid, func(tx pgx.Tx, _ projects.Scope, _ Eligibility) error {
		ns, e := s.repo.notices(ctx, tx, team, pid, nil, &a.UserID)
		if e != nil {
			return e
		}
		for _, n := range ns {
			c, e := s.repo.caseByID(ctx, tx, team, pid, n.CaseID, false)
			if e != nil {
				return e
			}
			v, e := s.repo.version(ctx, tx, c, n.CaseVersionID)
			if e != nil {
				return e
			}
			if v.Confirmation == nil || v.Confirmation.ID != n.ConfirmationID {
				return projects.NotFound()
			}
			out = append(out, Result{n, *v.Confirmation, v.Summary, v.Shares})
		}
		return nil
	})
	return
}

// token is a private transport credential, never part of the public input,
// idempotency hash, completed-command audit, response or error log.
func (s *Service) Acknowledge(ctx context.Context, a reqctx.Identity, team, pid, cid, notice uuid.UUID, in ReceiptInput, token string) (CommandResult, error) {
	cmd := Command{in.ExpectedVersion, in.ClientRequestID, "recipient explicitly acknowledged receipt"}
	if e := normalizeCommand(&cmd, false); e != nil {
		return CommandResult{}, e
	}
	if !in.Received || in.CaseVersionID == uuid.Nil || in.ConfirmationID == uuid.Nil || !validDigest(in.ContentDigest) {
		return CommandResult{}, invalid("receipt", "received=true and the exact displayed version, confirmation and digest are required")
	}
	return s.caseCommand(ctx, a, team, pid, cid, "receipt", cmd, struct {
		Notice uuid.UUID
		Input  ReceiptInput
	}{notice, in}, func(tx pgx.Tx, c Case, _ Eligibility) error {
		ok, e := s.repo.credential(ctx, tx, a, token)
		if e != nil {
			return e
		}
		if !ok {
			return httpx.Unauthorized("the session is invalid or has expired")
		}
		var recipient uuid.UUID
		var version, confirmation uuid.UUID
		var channel string
		e = tx.QueryRow(ctx, `SELECT recipient_id,case_version_id,confirmation_id,channel FROM contribution_notifications WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND case_id=$4 AND id=$5`, team, pid, c.TaskID, c.ID, notice).Scan(&recipient, &version, &confirmation, &channel)
		if e != nil {
			return e
		}
		if recipient != a.UserID {
			return projects.Forbidden()
		}
		if version != in.CaseVersionID || confirmation != in.ConfirmationID || channel != "authenticated_ack" {
			return httpx.Conflict("the receipt does not match the prepared result")
		}
		f, e := s.repo.confirmation(ctx, tx, c, in.CaseVersionID)
		if e != nil {
			return e
		}
		if f == nil || f.ID != in.ConfirmationID {
			return projects.NotFound()
		}
		if f.VersionDigest != in.ContentDigest {
			return httpx.Conflict("the displayed result digest does not match")
		}
		return nil
	}, func(tx pgx.Tx, c Case, request string) (uuid.UUID, error) {
		var id uuid.UUID
		e := tx.QueryRow(ctx, `SELECT id FROM contribution_attestations WHERE team_id=$1 AND project_id=$2 AND case_id=$3 AND notice_id=$4 AND subject_id=$5 AND purpose='delivery'`, team, pid, c.ID, notice, a.UserID).Scan(&id)
		if e == nil {
			return id, nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return uuid.Nil, e
		}
		return s.repo.receipt(ctx, tx, c, notice, a.UserID, in, token, request)
	})
}

// ReceiveIdentityRequest never returns team data or verification outcome. A
// departed subject may authenticate their own statement, without being granted
// access to results, membership lists, cases or normal acknowledgment routes.
func (s *Service) ReceiveIdentityRequest(ctx context.Context, a reqctx.Identity, in IdentityRequestInput, token string) (out Intake, err error) {
	if e := authenticated(a); e != nil {
		return out, e
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.ClientRequestID == uuid.Nil || in.TeamID == uuid.Nil || in.ProjectID == uuid.Nil || in.CaseID == uuid.Nil || in.CaseVersionID == uuid.Nil || in.ConfirmationID == uuid.Nil {
		return out, invalid("request", "all reference IDs are required")
	}
	if in.Reason == "" || !bounded(in.Reason, 4000) {
		return out, invalid("reason", "must contain 1–4000 characters")
	}
	out = Intake{in.ClientRequestID, true}
	ctx = requestContext(ctx)
	tx, e := s.repo.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if e != nil {
		return out, safeError(e)
	}
	defer tx.Rollback(ctx)
	var id uuid.UUID
	e = tx.QueryRow(ctx, `SELECT id FROM teams WHERE id=$1 FOR UPDATE`, in.TeamID).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, nil
	}
	if e != nil {
		return out, safeError(e)
	}
	ok, e := s.repo.credential(ctx, tx, a, token)
	if e != nil {
		return out, safeError(e)
	}
	if !ok {
		return out, httpx.Unauthorized("the session is invalid or has expired")
	}
	var task uuid.UUID
	e = tx.QueryRow(ctx, `SELECT c.task_id FROM contribution_cases c JOIN projects p ON p.team_id=c.team_id AND p.id=c.project_id AND p.status='active' JOIN project_members pm ON pm.team_id=c.team_id AND pm.project_id=c.project_id AND pm.user_id=$7 JOIN contribution_confirmations f ON f.team_id=c.team_id AND f.project_id=c.project_id AND f.task_id=c.task_id AND f.case_id=c.id AND f.case_version_id=c.effective_confirmed_version_id WHERE c.team_id=$1 AND c.project_id=$2 AND c.id=$3 AND f.case_version_id=$4 AND f.id=$5 AND EXISTS(SELECT 1 FROM users WHERE id=$7) AND $6::uuid IS NOT NULL FOR UPDATE OF c`, in.TeamID, in.ProjectID, in.CaseID, in.CaseVersionID, in.ConfirmationID, in.ClientRequestID, a.UserID).Scan(&task)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, nil
	}
	if e != nil {
		return out, safeError(e)
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contribution_attestations WHERE id=$1)`, in.ClientRequestID).Scan(&exists); e != nil {
		return out, safeError(e)
	}
	if !exists {
		_, e = tx.Exec(ctx, `INSERT INTO contribution_attestations(id,team_id,project_id,task_id,case_id,case_version_id,confirmation_id,purpose,subject_id,actor_id,statement,content_digest,proof_token,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,'dispute_request',$8,$8,$9,$10,$11,$12)`, in.ClientRequestID, in.TeamID, in.ProjectID, task, in.CaseID, in.CaseVersionID, in.ConfirmationID, a.UserID, in.Reason, digestString(in.Reason), token, reqctx.RequestID(ctx))
		if e != nil {
			return out, safeError(e)
		}
	}
	return out, safeError(tx.Commit(ctx))
}

func (s *Service) IdentityRequestEvidence(ctx context.Context, a reqctx.Identity, team, pid, cid, id uuid.UUID) (out IdentityRequestEvidence, err error) {
	err = s.readCase(ctx, a, team, pid, cid, func(tx pgx.Tx, c Case, _ projects.Scope, el Eligibility) error {
		if !el.CanEditCases {
			return projects.Forbidden()
		}
		var e error
		out, e = s.repo.identityEvidence(ctx, tx, c, id)
		return e
	})
	return
}
