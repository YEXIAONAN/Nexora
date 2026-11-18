package contributions

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func readJSON[T any](row pgx.Row) (out T, err error) {
	var b []byte
	if err = row.Scan(&b); err != nil {
		return
	}
	err = json.Unmarshal(b, &out)
	return
}
func readList[T any](ctx context.Context, tx pgx.Tx, q string, args ...any) ([]T, error) {
	out := []T{}
	rows, e := tx.Query(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := readJSON[T](rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) session(ctx context.Context, tx pgx.Tx, a reqctx.Identity) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>clock_timestamp())`, a.SessionID, a.UserID).Scan(&ok)
	return ok, e
}
func (r *Repository) credential(ctx context.Context, tx pgx.Tx, a reqctx.Identity, token string) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=$1 AND user_id=$2 AND token_hash=encode(sha256(convert_to($3,'UTF8')),'hex') AND revoked_at IS NULL AND expires_at>clock_timestamp())`, a.SessionID, a.UserID, token).Scan(&ok)
	return ok, e
}
func (r *Repository) eligibility(ctx context.Context, tx pgx.Tx, team, pid, user uuid.UUID) (Eligibility, error) {
	var x Eligibility
	e := tx.QueryRow(ctx, `SELECT contribution_appraiser($1,$2,$3),contribution_editor($1,$2,$3),COALESCE((SELECT version FROM contribution_appraisers WHERE team_id=$1 AND project_id=$2 AND user_id=$3),0)`, team, pid, user).Scan(&x.ExplicitAppraiser, &x.CanEditCases, &x.AuthorizationVersion)
	return x, e
}
func (r *Repository) appraisers(ctx context.Context, tx pgx.Tx, team, pid uuid.UUID) ([]Appraiser, error) {
	return readList[Appraiser](ctx, tx, `SELECT to_jsonb(a)||jsonb_build_object('qualified',contribution_appraiser(a.team_id,a.project_id,a.user_id)) FROM contribution_appraisers a WHERE a.team_id=$1 AND a.project_id=$2 ORDER BY a.user_id`, team, pid)
}
func (r *Repository) caseByID(ctx context.Context, tx pgx.Tx, team, pid, id uuid.UUID, lock bool) (Case, error) {
	q := `SELECT to_jsonb(c) FROM contribution_cases c WHERE c.team_id=$1 AND c.project_id=$2 AND c.id=$3`
	if lock {
		q += ` FOR UPDATE`
	}
	return readJSON[Case](tx.QueryRow(ctx, q, team, pid, id))
}

const relatedCase = `(contribution_editor(c.team_id,c.project_id,$3) OR c.created_by=$3 OR EXISTS(SELECT 1 FROM contribution_case_versions v WHERE v.case_id=c.id AND v.proposed_by=$3) OR EXISTS(SELECT 1 FROM contribution_shares s WHERE s.case_id=c.id AND s.user_id=$3) OR EXISTS(SELECT 1 FROM contribution_disputes d WHERE d.case_id=c.id AND (d.raised_by=$3 OR d.on_behalf_of=$3)))`

func (r *Repository) visible(ctx context.Context, tx pgx.Tx, c Case, user uuid.UUID) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contribution_cases c WHERE c.team_id=$1 AND c.project_id=$2 AND c.id=$4 AND `+relatedCase+`)`, c.TeamID, c.ProjectID, user, c.ID).Scan(&ok)
	return ok, e
}
func (r *Repository) cases(ctx context.Context, tx pgx.Tx, team, pid, user uuid.UUID, pending bool) ([]Case, error) {
	q := `SELECT to_jsonb(c) FROM contribution_cases c WHERE c.team_id=$1 AND c.project_id=$2 AND ` + relatedCase
	if pending {
		q += ` AND EXISTS(SELECT 1 FROM contribution_case_versions v WHERE v.id=c.latest_version_id AND v.status='pending_review' AND contribution_independent(v.id,$3)) AND NOT contribution_open_disputes(c.id) AND EXISTS(SELECT 1 FROM projects p WHERE p.team_id=c.team_id AND p.id=c.project_id AND p.status='active')`
	}
	return readList[Case](ctx, tx, q+` ORDER BY c.created_at,c.id`, team, pid, user)
}
func (r *Repository) shares(ctx context.Context, tx pgx.Tx, c Case, version uuid.UUID) ([]Share, error) {
	return readList[Share](ctx, tx, `SELECT to_jsonb(s) FROM contribution_shares s WHERE s.team_id=$1 AND s.project_id=$2 AND s.task_id=$3 AND s.case_id=$4 AND s.case_version_id=$5 ORDER BY s.user_id`, c.TeamID, c.ProjectID, c.TaskID, c.ID, version)
}
func (r *Repository) confirmation(ctx context.Context, tx pgx.Tx, c Case, version uuid.UUID) (*Confirmation, error) {
	f, e := readJSON[Confirmation](tx.QueryRow(ctx, `SELECT to_jsonb(f) FROM contribution_confirmations f WHERE f.team_id=$1 AND f.project_id=$2 AND f.task_id=$3 AND f.case_id=$4 AND f.case_version_id=$5`, c.TeamID, c.ProjectID, c.TaskID, c.ID, version))
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return &f, nil
}
func (r *Repository) version(ctx context.Context, tx pgx.Tx, c Case, id uuid.UUID) (Version, error) {
	v, e := readJSON[Version](tx.QueryRow(ctx, `SELECT to_jsonb(v) FROM contribution_case_versions v WHERE v.team_id=$1 AND v.project_id=$2 AND v.task_id=$3 AND v.case_id=$4 AND v.id=$5`, c.TeamID, c.ProjectID, c.TaskID, c.ID, id))
	if e != nil {
		return v, e
	}
	if v.Shares, e = r.shares(ctx, tx, c, id); e != nil {
		return v, e
	}
	for _, s := range v.Shares {
		v.TotalBPS += s.ShareBPS
	}
	if v.Confirmation, e = r.confirmation(ctx, tx, c, id); e != nil {
		return v, e
	}
	e = tx.QueryRow(ctx, `SELECT $2 AND contribution_complete_shares($1)`, id, v.Status == "draft").Scan(&v.ReadyToSubmit)
	return v, e
}
func (r *Repository) versions(ctx context.Context, tx pgx.Tx, c Case) ([]Version, error) {
	vs, e := readList[Version](ctx, tx, `SELECT to_jsonb(v) FROM contribution_case_versions v WHERE v.team_id=$1 AND v.project_id=$2 AND v.task_id=$3 AND v.case_id=$4 ORDER BY v.version`, c.TeamID, c.ProjectID, c.TaskID, c.ID)
	if e != nil {
		return nil, e
	}
	for i := range vs {
		if vs[i], e = r.version(ctx, tx, c, vs[i].ID); e != nil {
			return nil, e
		}
	}
	return vs, nil
}
func (r *Repository) independent(ctx context.Context, tx pgx.Tx, version, user uuid.UUID) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT COALESCE(contribution_independent($1,$2),false)`, version, user).Scan(&ok)
	return ok, e
}
func (r *Repository) beneficiary(ctx context.Context, tx pgx.Tx, version, user uuid.UUID) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contribution_shares WHERE case_version_id=$1 AND user_id=$2)`, version, user).Scan(&ok)
	return ok, e
}
func (r *Repository) memberExists(ctx context.Context, tx pgx.Tx, team, pid, user uuid.UUID) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE team_id=$1 AND project_id=$2 AND user_id=$3)`, team, pid, user).Scan(&ok)
	return ok, e
}
func (r *Repository) accepted(ctx context.Context, tx pgx.Tx, team, pid, task, review uuid.UUID) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tasks t JOIN task_reviews r ON r.team_id=t.team_id AND r.project_id=t.project_id AND r.task_id=t.id AND r.id=t.accepted_review_id AND r.submission_version_id=t.current_submission_version_id WHERE t.team_id=$1 AND t.project_id=$2 AND t.id=$3 AND r.id=$4 AND t.status='completed' AND r.decision='approve')`, team, pid, task, review).Scan(&ok)
	return ok, e
}
func (r *Repository) evidence(ctx context.Context, tx pgx.Tx, c Case) (AcceptanceEvidence, error) {
	out, e := readJSON[AcceptanceEvidence](tx.QueryRow(ctx, `SELECT jsonb_build_object('task_id',t.id,'task_title',t.title,'task_owner_id',t.owner_id,'submission_version_id',v.id,'author_id',v.author_id,'summary',v.summary,'evidence_references',v.evidence_references,'submitted_at',v.submitted_at,'review_id',r.id,'reviewer_id',r.reviewer_id,'decision',r.decision,'comment',r.comment,'reviewed_at',r.reviewed_at) FROM tasks t JOIN task_submission_versions v ON v.team_id=t.team_id AND v.project_id=t.project_id AND v.task_id=t.id AND v.id=t.current_submission_version_id JOIN task_reviews r ON r.team_id=t.team_id AND r.project_id=t.project_id AND r.task_id=t.id AND r.submission_version_id=v.id AND r.id=t.accepted_review_id AND r.decision='approve' WHERE t.team_id=$1 AND t.project_id=$2 AND t.id=$3 AND v.id=$4 AND r.id=$5 AND t.status='completed'`, c.TeamID, c.ProjectID, c.TaskID, c.SubmissionVersionID, c.ReviewID))
	if e != nil {
		return out, e
	}
	out.Attachments, e = readList[EvidenceAttachment](ctx, tx, `SELECT jsonb_build_object('id',id,'uploader_id',uploader_id,'file_name',file_name,'byte_size',byte_size,'sha256',sha256) FROM submission_attachments WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND submission_version_id=$4 AND status='bound' ORDER BY id`, c.TeamID, c.ProjectID, c.TaskID, c.SubmissionVersionID)
	return out, e
}
func (r *Repository) disputes(ctx context.Context, tx pgx.Tx, c Case) ([]Dispute, error) {
	return readList[Dispute](ctx, tx, `SELECT to_jsonb(d)||jsonb_build_object('resolution',(SELECT to_jsonb(r) FROM contribution_dispute_resolutions r WHERE r.dispute_id=d.id)) FROM contribution_disputes d WHERE d.team_id=$1 AND d.project_id=$2 AND d.task_id=$3 AND d.case_id=$4 ORDER BY d.created_at,d.id`, c.TeamID, c.ProjectID, c.TaskID, c.ID)
}
func (r *Repository) notices(ctx context.Context, tx pgx.Tx, team, pid uuid.UUID, caseID, user *uuid.UUID) ([]Notice, error) {
	// Explicit projection: never select receipt session or transient proof fields.
	return readList[Notice](ctx, tx, `SELECT to_jsonb(n)||jsonb_build_object('receipt',(SELECT jsonb_build_object('id',a.id,'notice_id',a.notice_id,'case_version_id',a.case_version_id,'confirmation_id',a.confirmation_id,'subject_id',a.subject_id,'content_digest',a.content_digest,'verified_at',a.verified_at,'sent_at',a.sent_at,'acknowledged_at',a.acknowledged_at) FROM contribution_attestations a WHERE a.notice_id=n.id AND a.purpose='delivery')) FROM contribution_notifications n WHERE n.team_id=$1 AND n.project_id=$2 AND ($3::uuid IS NULL OR n.case_id=$3) AND ($4::uuid IS NULL OR n.recipient_id=$4) ORDER BY n.prepared_at,n.id`, team, pid, caseID, user)
}
func (r *Repository) history(ctx context.Context, tx pgx.Tx, c Case) (History, error) {
	var h History
	var e error
	if h.Versions, e = r.versions(ctx, tx, c); e != nil {
		return h, e
	}
	if h.Events, e = readList[Event](ctx, tx, `SELECT to_jsonb(e) FROM contribution_case_events e WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND case_id=$4 ORDER BY created_at,id`, c.TeamID, c.ProjectID, c.TaskID, c.ID); e != nil {
		return h, e
	}
	if h.Adjustments, e = readList[Adjustment](ctx, tx, `SELECT to_jsonb(a) FROM contribution_share_adjustments a WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND case_id=$4 ORDER BY created_at,id`, c.TeamID, c.ProjectID, c.TaskID, c.ID); e != nil {
		return h, e
	}
	if h.Disputes, e = r.disputes(ctx, tx, c); e != nil {
		return h, e
	}
	if h.Conflicts, e = readList[Conflict](ctx, tx, `SELECT to_jsonb(f) FROM contribution_conflicts f WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND case_id=$4 ORDER BY created_at,id`, c.TeamID, c.ProjectID, c.TaskID, c.ID); e != nil {
		return h, e
	}
	h.Notices, e = r.notices(ctx, tx, c.TeamID, c.ProjectID, &c.ID, nil)
	return h, e
}

// Completed command receipts reuse the existing append-only audit model. Team
// serialization makes lookup + mutation + receipt atomic without a second store.
func (r *Repository) priorCommand(ctx context.Context, tx pgx.Tx, team, pid, actor, key uuid.UUID) (string, CommandResult, error) {
	var digest string
	var b []byte
	var result CommandResult
	e := tx.QueryRow(ctx, `SELECT details->>'input_digest',details->'result' FROM audit_events WHERE team_id=$1 AND target_id=$2 AND actor_id=$3 AND action='contribution.command_completed' AND details->>'client_request_id'=$4 ORDER BY created_at LIMIT 1`, team, pid, actor, key.String()).Scan(&digest, &b)
	if e == nil {
		e = json.Unmarshal(b, &result)
	}
	return digest, result, e
}
func (r *Repository) create(ctx context.Context, tx pgx.Tx, team, pid uuid.UUID, in CreateInput, actor uuid.UUID, request string) (uuid.UUID, error) {
	var id uuid.UUID
	e := tx.QueryRow(ctx, `SELECT contribution_create_case($1,$2,$3,$4,$5,$6)`, team, pid, in.TaskID, in.ReviewID, actor, request).Scan(&id)
	return id, e
}
func (r *Repository) grant(ctx context.Context, tx pgx.Tx, team, pid, user, actor uuid.UUID, in Command, revoke bool, request string) error {
	_, e := tx.Exec(ctx, `SELECT contribution_set_appraiser($1,$2,$3,$4,$5,$6,$7,$8)`, team, pid, user, actor, in.Reason, request, in.ExpectedVersion, revoke)
	return e
}
func (r *Repository) setShare(ctx context.Context, tx pgx.Tx, c Case, user, actor uuid.UUID, in ShareInput, request string) error {
	refs, e := json.Marshal(in.EvidenceRefs)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `SELECT contribution_set_share($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, c.TeamID, c.ProjectID, c.ID, in.CaseVersionID, user, in.ShareBPS, in.Basis, in.Statement, refs, actor, in.ExpectedVersion, in.Reason, request)
	return e
}
func (r *Repository) saveDraft(ctx context.Context, tx pgx.Tx, c Case, actor uuid.UUID, in DraftInput, request string) error {
	b, e := json.Marshal(in.EvidenceSnapshot)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE contribution_case_versions SET summary=$6,evidence_snapshot=$7,changed_by=$8,change_reason=$9,request_id=$10 WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND case_id=$4 AND id=$5`, c.TeamID, c.ProjectID, c.TaskID, c.ID, in.CaseVersionID, in.Summary, b, actor, in.Reason, request)
	return e
}
func (r *Repository) transition(ctx context.Context, tx pgx.Tx, c Case, actor uuid.UUID, in VersionCommand, to, request string) error {
	_, e := tx.Exec(ctx, `SELECT contribution_transition($1,$2,$3,$4,$5,$6,$7,$8)`, c.TeamID, c.ProjectID, c.ID, actor, in.ExpectedVersion, to, in.Reason, request)
	return e
}
func (r *Repository) confirm(ctx context.Context, tx pgx.Tx, c Case, actor uuid.UUID, in VersionCommand, request string) (uuid.UUID, error) {
	var id uuid.UUID
	e := tx.QueryRow(ctx, `SELECT contribution_confirm($1,$2,$3,$4,$5,$6,$7)`, c.TeamID, c.ProjectID, c.ID, actor, in.ExpectedVersion, in.Reason, request).Scan(&id)
	return id, e
}
func (r *Repository) revise(ctx context.Context, tx pgx.Tx, c Case, actor uuid.UUID, in VersionCommand, request string) (uuid.UUID, error) {
	var id uuid.UUID
	e := tx.QueryRow(ctx, `SELECT contribution_revise($1,$2,$3,$4,$5,$6,$7)`, c.TeamID, c.ProjectID, c.ID, actor, in.ExpectedVersion, in.Reason, request).Scan(&id)
	return id, e
}
func (r *Repository) dispute(ctx context.Context, tx pgx.Tx, c Case, actor uuid.UUID, in DisputeInput, request string) (uuid.UUID, error) {
	var id uuid.UUID
	e := tx.QueryRow(ctx, `SELECT contribution_raise_dispute($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, c.TeamID, c.ProjectID, c.ID, in.ConfirmationID, actor, in.ExpectedVersion, in.Reason, request, in.OnBehalfOf, in.IdentityRequestID).Scan(&id)
	return id, e
}
func (r *Repository) resolve(ctx context.Context, tx pgx.Tx, c Case, dispute, actor uuid.UUID, in ResolutionInput, request string) (uuid.UUID, error) {
	var id uuid.UUID
	e := tx.QueryRow(ctx, `SELECT contribution_resolve($1,$2,$3,$4,$5,$6,$7,$8,$9)`, c.TeamID, c.ProjectID, c.ID, dispute, actor, in.ExpectedVersion, in.Decision, in.Reason, request).Scan(&id)
	return id, e
}
func (r *Repository) conflict(ctx context.Context, tx pgx.Tx, c Case, actor uuid.UUID, in ConflictInput, request string) (uuid.UUID, error) {
	var id uuid.UUID
	e := tx.QueryRow(ctx, `INSERT INTO contribution_conflicts(team_id,project_id,task_id,case_id,user_id,kind,details,recorded_by,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, c.TeamID, c.ProjectID, c.TaskID, c.ID, in.UserID, in.Kind, in.Details, actor, request).Scan(&id)
	return id, e
}
func (r *Repository) notice(ctx context.Context, tx pgx.Tx, c Case, actor uuid.UUID, in NoticeInput, request string) (uuid.UUID, error) {
	var id uuid.UUID
	e := tx.QueryRow(ctx, `INSERT INTO contribution_notifications(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,recipient_id,channel,created_by,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,'authenticated_ack',$8,$9) RETURNING id`, c.TeamID, c.ProjectID, c.TaskID, c.ID, in.CaseVersionID, in.ConfirmationID, in.RecipientID, actor, request).Scan(&id)
	return id, e
}
func (r *Repository) receipt(ctx context.Context, tx pgx.Tx, c Case, notice, actor uuid.UUID, in ReceiptInput, token, request string) (uuid.UUID, error) {
	var id uuid.UUID
	e := tx.QueryRow(ctx, `INSERT INTO contribution_attestations(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,purpose,notice_id,subject_id,actor_id,content_digest,proof_token,request_id) VALUES($1,$2,$3,$4,$5,$6,'delivery',$7,$8,$8,$9,$10,$11) RETURNING id`, c.TeamID, c.ProjectID, c.TaskID, c.ID, in.CaseVersionID, in.ConfirmationID, notice, actor, in.ContentDigest, token, request).Scan(&id)
	return id, e
}
