package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(p *pgxpool.Pool) *Repository { return &Repository{p} }
func (r *Repository) CanReview(ctx context.Context, tx pgx.Tx, team, pid, user uuid.UUID) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT phase04_can_review($1,$2,$3)`, team, pid, user).Scan(&ok)
	return ok, e
}

const reviewColumns = `r.id,r.team_id,r.project_id,r.task_id,r.submission_version_id,r.reviewer_id,COALESCE(NULLIF(u.display_name,''),u.username),r.decision,r.comment,r.reviewed_at,r.request_id,r.task_version`

func scanReview(row pgx.Row) (r Review, e error) {
	e = row.Scan(&r.ID, &r.TeamID, &r.ProjectID, &r.TaskID, &r.SubmissionVersionID, &r.ReviewerID, &r.ReviewerName, &r.Decision, &r.Comment, &r.ReviewedAt, &r.RequestID, &r.TaskVersion)
	return
}
func (r *Repository) reviews(ctx context.Context, tx pgx.Tx, team, pid, task uuid.UUID) (out []Review, err error) {
	out = []Review{}
	rows, e := tx.Query(ctx, `SELECT `+reviewColumns+` FROM task_reviews r JOIN users u ON u.id=r.reviewer_id WHERE r.team_id=$1 AND r.project_id=$2 AND r.task_id=$3 ORDER BY r.reviewed_at,r.id`, team, pid, task)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanReview(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) version(ctx context.Context, tx pgx.Tx, team, pid, task, id uuid.UUID) (out Version, err error) {
	var refs []byte
	err = tx.QueryRow(ctx, `SELECT v.id,v.team_id,v.project_id,v.task_id,v.submission_id,v.version,v.author_id,COALESCE(NULLIF(u.display_name,''),u.username),v.summary,v.evidence_references,v.submitted_at,v.based_on_task_version FROM task_submission_versions v JOIN users u ON u.id=v.author_id WHERE v.team_id=$1 AND v.project_id=$2 AND v.task_id=$3 AND v.id=$4`, team, pid, task, id).Scan(&out.ID, &out.TeamID, &out.ProjectID, &out.TaskID, &out.SubmissionID, &out.Version, &out.AuthorID, &out.AuthorName, &out.Summary, &refs, &out.SubmittedAt, &out.BasedOnTaskVersion)
	if err != nil {
		return
	}
	if err = json.Unmarshal(refs, &out.References); err != nil {
		return
	}
	out.Attachments, err = r.attachments(ctx, tx, team, pid, task, &id)
	if err != nil {
		return
	}
	review, e := scanReview(tx.QueryRow(ctx, `SELECT `+reviewColumns+` FROM task_reviews r JOIN users u ON u.id=r.reviewer_id WHERE r.team_id=$1 AND r.project_id=$2 AND r.task_id=$3 AND r.submission_version_id=$4`, team, pid, task, id))
	if e == nil {
		out.Review = &review
	} else if !errors.Is(e, pgx.ErrNoRows) {
		err = e
	}
	return
}
func (r *Repository) versions(ctx context.Context, tx pgx.Tx, team, pid, task uuid.UUID) (out []Version, err error) {
	rows, e := tx.Query(ctx, `SELECT id FROM task_submission_versions WHERE team_id=$1 AND project_id=$2 AND task_id=$3 ORDER BY version DESC`, team, pid, task)
	if e != nil {
		return nil, e
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out = []Version{}
	for _, id := range ids {
		v, e := r.version(ctx, tx, team, pid, task, id)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return
}

const attachmentColumns = `id,team_id,project_id,task_id,uploader_id,submission_version_id,file_name,media_type,byte_size,sha256,status,created_at,expires_at,removed_at,storage_key`

func scanAttachment(row pgx.Row) (a Attachment, e error) {
	e = row.Scan(&a.ID, &a.TeamID, &a.ProjectID, &a.TaskID, &a.UploaderID, &a.SubmissionVersionID, &a.FileName, &a.MediaType, &a.ByteSize, &a.SHA256, &a.Status, &a.CreatedAt, &a.ExpiresAt, &a.RemovedAt, &a.storageKey)
	return
}
func (r *Repository) attachments(ctx context.Context, tx pgx.Tx, team, pid, task uuid.UUID, version *uuid.UUID) (out []Attachment, err error) {
	rows, e := tx.Query(ctx, `SELECT `+attachmentColumns+` FROM submission_attachments WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND submission_version_id=$4 AND status='bound' ORDER BY created_at,id`, team, pid, task, version)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out = []Attachment{}
	for rows.Next() {
		a, e := scanAttachment(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (r *Repository) reviewers(ctx context.Context, tx pgx.Tx, team, pid uuid.UUID) (out []Reviewer, err error) {
	rows, e := tx.Query(ctx, `SELECT pm.user_id,u.username,u.display_name,p.lead_id=pm.user_id,phase04_can_review(pm.team_id,pm.project_id,pm.user_id),COALESCE(r.version,0),r.granted_by,r.granted_at,COALESCE(r.reason,'') FROM project_members pm JOIN projects p ON p.team_id=pm.team_id AND p.id=pm.project_id JOIN users u ON u.id=pm.user_id LEFT JOIN project_reviewers r ON r.team_id=pm.team_id AND r.project_id=pm.project_id AND r.user_id=pm.user_id WHERE pm.team_id=$1 AND pm.project_id=$2 AND (r.user_id IS NOT NULL OR pm.user_id=p.lead_id) ORDER BY p.lead_id=pm.user_id DESC,u.username`, team, pid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out = []Reviewer{}
	for rows.Next() {
		var v Reviewer
		if e = rows.Scan(&v.UserID, &v.Username, &v.DisplayName, &v.DefaultLead, &v.Active, &v.Version, &v.GrantedBy, &v.GrantedAt, &v.Reason); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
