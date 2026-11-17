package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/tasks"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"strings"
	"unicode/utf8"
)

type Service struct {
	store    *Store
	repo     *Repository
	projects *projects.Service
	tasks    *tasks.Service
}

func NewService(r *Repository, p *projects.Service, t *tasks.Service) *Service {
	s := &Service{repo: r, projects: p, tasks: t}
	t.SetReviewAccess(r)
	return s
}
func (s *Service) Versions(ctx context.Context, a reqctx.Identity, team, pid, task uuid.UUID) (out []Version, err error) {
	err = s.tasks.Workflow(ctx, a, team, pid, task, false, func(w *tasks.Workflow) error {
		if e := w.RequireRead(); e != nil {
			return e
		}
		var e error
		out, e = s.repo.versions(ctx, w.Tx, team, pid, task)
		return e
	})
	return
}
func (s *Service) Version(ctx context.Context, a reqctx.Identity, team, pid, task, id uuid.UUID) (out Version, err error) {
	err = s.tasks.Workflow(ctx, a, team, pid, task, false, func(w *tasks.Workflow) error {
		if e := w.RequireRead(); e != nil {
			return e
		}
		var e error
		out, e = s.repo.version(ctx, w.Tx, team, pid, task, id)
		if errors.Is(e, pgx.ErrNoRows) {
			return projects.NotFound()
		}
		return e
	})
	return
}
func (s *Service) Reviews(ctx context.Context, a reqctx.Identity, team, pid, task uuid.UUID) (out []Review, err error) {
	err = s.tasks.Workflow(ctx, a, team, pid, task, false, func(w *tasks.Workflow) error {
		if e := w.RequireRead(); e != nil {
			return e
		}
		var e error
		out, e = s.repo.reviews(ctx, w.Tx, team, pid, task)
		return e
	})
	return
}
func (s *Service) Submit(ctx context.Context, a reqctx.Identity, team, pid, task uuid.UUID, in SubmitInput) (out Version, err error) {
	hash, e := normalize(&in)
	if e != nil {
		return out, e
	}
	err = s.tasks.Workflow(ctx, a, team, pid, task, true, func(w *tasks.Workflow) error {
		if e := w.RequireSubmit(false); e != nil {
			return e
		}
		var prior, author uuid.UUID
		var priorHash string
		e := w.Tx.QueryRow(ctx, `SELECT id,author_id,content_hash FROM task_submission_versions WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND client_request_id=$4`, team, pid, task, in.ClientRequestID).Scan(&prior, &author, &priorHash)
		if e == nil {
			if author != a.UserID || priorHash != hash {
				return httpx.Conflict("submission retry key was used for different content")
			}
			out, e = s.repo.version(ctx, w.Tx, team, pid, task, prior)
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e = w.RequireSubmit(true); e != nil {
			return e
		}
		if e = projects.CheckVersion(in.ExpectedVersion, w.Task.Version); e != nil {
			return e
		}
		// Every referenced file must be private, current, staged by this author in
		// this exact task scope. Binding is atomic with the new version.
		var bytes int64
		for _, id := range in.AttachmentIDs {

			file, e := scanAttachment(w.Tx.QueryRow(ctx, `SELECT `+attachmentColumns+` FROM submission_attachments WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND id=$4 AND uploader_id=$5 AND status='staged' AND removed_at IS NULL AND expires_at>clock_timestamp() FOR UPDATE`, team, pid, task, id, a.UserID))
			if errors.Is(e, pgx.ErrNoRows) {
				return projects.NotFound()
			}
			if e != nil {
				return e
			}
			if s.store == nil {
				return httpx.Unavailable("private attachment storage is unavailable")
			}
			content, e := s.store.open(file)
			if e != nil {
				return httpx.Unavailable("staged attachment content is unavailable; upload again")
			}
			content.Close()
			bytes += file.ByteSize

		}
		if bytes > MaxVersionBytes {
			return httpx.ValidationFailed(map[string]string{"attachment_ids": "total attachments exceed 25 MiB"})
		}
		var sid uuid.UUID
		e = w.Tx.QueryRow(ctx, `SELECT id FROM task_submissions WHERE team_id=$1 AND project_id=$2 AND task_id=$3`, team, pid, task).Scan(&sid)
		if errors.Is(e, pgx.ErrNoRows) {
			sid = uuid.New()
			_, e = w.Tx.Exec(ctx, `INSERT INTO task_submissions(id,team_id,project_id,task_id) VALUES($1,$2,$3,$4)`, sid, team, pid, task)
		}
		if e != nil {
			return e
		}
		id := uuid.New()
		refs, e := json.Marshal(in.References)
		if e != nil {
			return e
		}
		_, e = w.Tx.Exec(ctx, `INSERT INTO task_submission_versions(id,team_id,project_id,task_id,submission_id,version,author_id,summary,evidence_references,client_request_id,content_hash,based_on_task_version,request_id) SELECT $1,$2,$3,$4,$5,COALESCE(max(version),0)+1,$6,$7,$8,$9,$10,$11,$12 FROM task_submission_versions WHERE team_id=$2 AND project_id=$3 AND task_id=$4`, id, team, pid, task, sid, a.UserID, in.Summary, refs, in.ClientRequestID, hash, w.Task.Version, reqctx.RequestID(ctx))
		if e != nil {
			return e
		}
		for _, aid := range in.AttachmentIDs {
			if _, e = w.Tx.Exec(ctx, `UPDATE submission_attachments SET status='bound',submission_version_id=$5 WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND id=$4`, team, pid, task, aid, id); e != nil {
				return e
			}
		}
		if e = w.SubmitVersion(id); e != nil {
			return e
		}
		if e = s.projects.Event(ctx, w.Tx, a, team, id, "submission.created", "submission_version", map[string]any{"project_id": pid, "task_id": task, "client_request_id": in.ClientRequestID, "attachment_count": len(in.AttachmentIDs)}); e != nil {
			return e
		}
		out, e = s.repo.version(ctx, w.Tx, team, pid, task, id)
		return e
	})
	return
}
func (s *Service) Review(ctx context.Context, a reqctx.Identity, team, pid, task, version uuid.UUID, in ReviewInput) (out Review, err error) {
	in.Comment = strings.TrimSpace(in.Comment)
	if (in.Decision != "approve" && in.Decision != "reject") || utf8.RuneCountInString(in.Comment) < 1 || utf8.RuneCountInString(in.Comment) > 4000 || strings.ContainsRune(in.Comment, 0) {
		return out, httpx.ValidationFailed(map[string]string{"review": "provide approve/reject and a comment of 1–4000 characters"})
	}
	err = s.tasks.Workflow(ctx, a, team, pid, task, true, func(w *tasks.Workflow) error {
		if e := w.RequireRead(); e != nil {
			return e
		}
		v, e := s.repo.version(ctx, w.Tx, team, pid, task, version)
		if errors.Is(e, pgx.ErrNoRows) {
			return projects.NotFound()
		}
		if e != nil {
			return e
		}
		ok, e := s.repo.CanReview(ctx, w.Tx, team, pid, a.UserID)
		if e != nil {
			return e
		}
		if !ok || v.AuthorID == a.UserID {
			return projects.Forbidden()
		}
		if e = projects.CheckVersion(in.ExpectedVersion, w.Task.Version); e != nil {
			return e
		}
		if w.Task.Status != tasks.AwaitingReview || w.Task.CurrentSubmissionVersionID == nil || *w.Task.CurrentSubmissionVersionID != version {
			return httpx.Conflict("only the current awaiting version can be reviewed")
		}
		out = Review{ID: uuid.New(), TeamID: team, ProjectID: pid, TaskID: task, SubmissionVersionID: version, ReviewerID: a.UserID, Decision: in.Decision, Comment: in.Comment, RequestID: reqctx.RequestID(ctx), TaskVersion: w.Task.Version}
		e = w.Tx.QueryRow(ctx, `INSERT INTO task_reviews(id,team_id,project_id,task_id,submission_version_id,reviewer_id,decision,comment,request_id,task_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING reviewed_at`, out.ID, team, pid, task, version, a.UserID, in.Decision, in.Comment, out.RequestID, out.TaskVersion).Scan(&out.ReviewedAt)
		if e != nil {
			return e
		}
		if e = w.ApplyReview(version, out.ID, in.Decision, in.Comment); e != nil {
			return e
		}
		if e = w.Tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(display_name,''),username) FROM users WHERE id=$1`, a.UserID).Scan(&out.ReviewerName); e != nil {
			return e
		}
		return s.projects.Event(ctx, w.Tx, a, team, out.ID, "submission.reviewed", "task_review", map[string]any{"project_id": pid, "task_id": task, "submission_version_id": version, "decision": in.Decision, "comment": in.Comment})
	})
	return
}
func (s *Service) Reviewers(ctx context.Context, a reqctx.Identity, team, pid uuid.UUID) (out []Reviewer, err error) {
	err = s.projects.Within(ctx, a, team, pid, false, false, func(tx pgx.Tx, sc projects.Scope) error {
		ok, e := s.projects.Qualified(ctx, tx, team, pid, a.UserID)
		if e != nil {
			return e
		}
		if !ok {
			return projects.NotFound()
		}
		out, e = s.repo.reviewers(ctx, tx, team, pid)
		return e
	})
	return
}
func (s *Service) Grant(ctx context.Context, a reqctx.Identity, team, pid, user uuid.UUID, reason string) (out []Reviewer, err error) {
	if e := projects.ValidateReason(reason); e != nil {
		return nil, e
	}
	err = s.projects.Within(ctx, a, team, pid, true, true, func(tx pgx.Tx, sc projects.Scope) error {
		ok, e := s.projects.Qualified(ctx, tx, team, pid, user)
		if e != nil {
			return e
		}
		if !ok {
			return projects.NotFound()
		}
		if user == sc.Project.LeadID {
			return httpx.Conflict("project lead already has default review authority")
		}
		ok, e = s.repo.CanReview(ctx, tx, team, pid, user)
		if e != nil {
			return e
		}
		if ok {
			return httpx.Conflict("reviewer grant already active")
		}
		_, e = tx.Exec(ctx, `INSERT INTO project_reviewers(team_id,project_id,user_id,member_authorization_version,status,granted_by,reason) SELECT $1,$2,$3,authorization_version,'active',$4,$5 FROM project_members WHERE team_id=$1 AND project_id=$2 AND user_id=$3 ON CONFLICT(team_id,project_id,user_id) DO UPDATE SET member_authorization_version=EXCLUDED.member_authorization_version,status='active',granted_by=EXCLUDED.granted_by,granted_at=clock_timestamp(),reason=EXCLUDED.reason,version=project_reviewers.version+1`, team, pid, user, a.UserID, strings.TrimSpace(reason))
		if e != nil {
			return e
		}
		if e = s.projects.Event(ctx, tx, a, team, pid, "project.reviewer_granted", "project", map[string]any{"user_id": user, "reason": strings.TrimSpace(reason)}); e != nil {
			return e
		}
		out, e = s.repo.reviewers(ctx, tx, team, pid)
		return e
	})
	return
}
func (s *Service) Revoke(ctx context.Context, a reqctx.Identity, team, pid, user uuid.UUID, expected int, reason string) (out []Reviewer, err error) {
	if e := projects.ValidateReason(reason); e != nil {
		return nil, e
	}
	err = s.projects.Within(ctx, a, team, pid, true, true, func(tx pgx.Tx, sc projects.Scope) error {
		if user == sc.Project.LeadID {
			return httpx.Conflict("hand off project lead to remove default authority")
		}
		var version int
		var status string
		e := tx.QueryRow(ctx, `SELECT version,status FROM project_reviewers WHERE team_id=$1 AND project_id=$2 AND user_id=$3`, team, pid, user).Scan(&version, &status)
		if errors.Is(e, pgx.ErrNoRows) {
			return projects.NotFound()
		}
		if e != nil {
			return e
		}
		if e = projects.CheckVersion(expected, version); e != nil {
			return e
		}
		if status != "active" {
			return httpx.Conflict("grant already revoked")
		}
		_, e = tx.Exec(ctx, `UPDATE project_reviewers SET status='revoked',version=version+1,reason=$4 WHERE team_id=$1 AND project_id=$2 AND user_id=$3`, team, pid, user, strings.TrimSpace(reason))
		if e != nil {
			return e
		}
		if e = s.projects.Event(ctx, tx, a, team, pid, "project.reviewer_revoked", "project", map[string]any{"user_id": user, "reason": strings.TrimSpace(reason)}); e != nil {
			return e
		}
		out, e = s.repo.reviewers(ctx, tx, team, pid)
		return e
	})
	return
}
func (s *Service) Pending(ctx context.Context, a reqctx.Identity, team uuid.UUID) (out []Pending, err error) {
	out = []Pending{}
	err = s.projects.WithTeam(ctx, a, team, func(tx pgx.Tx, _ teams.Member) error {
		rows, e := tx.Query(ctx, `SELECT p.id,p.name,t.id,t.title,t.version,v.id,v.version,COALESCE(NULLIF(u.display_name,''),u.username),v.submitted_at FROM tasks t JOIN projects p ON p.team_id=t.team_id AND p.id=t.project_id JOIN task_submission_versions v ON v.team_id=t.team_id AND v.project_id=t.project_id AND v.task_id=t.id AND v.id=t.current_submission_version_id JOIN users u ON u.id=v.author_id WHERE t.team_id=$1 AND p.status='active' AND t.status='awaiting_review' AND v.author_id<>$2 AND phase04_can_review(t.team_id,t.project_id,$2) ORDER BY v.submitted_at,v.id`, team, a.UserID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v Pending
			if e = rows.Scan(&v.ProjectID, &v.ProjectName, &v.TaskID, &v.TaskTitle, &v.TaskVersion, &v.SubmissionVersionID, &v.Version, &v.AuthorName, &v.SubmittedAt); e != nil {
				return e
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return
}
