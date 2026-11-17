package submissions

import (
	"context"
	"errors"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"os"
	"time"
)

func (s *Service) SetStore(store *Store) { s.store = store }
func (s *Service) Upload(ctx context.Context, a reqctx.Identity, team, pid, task, key uuid.UUID, name string, body io.Reader) (out Attachment, err error) {
	if s.store == nil {
		return out, httpx.Unavailable("private attachment storage is unavailable")
	}
	if key == uuid.Nil {
		return out, httpx.ValidationFailed(map[string]string{"X-Nexora-Upload-Id": "a nonzero UUID is required"})
	}
	var createdKey string
	err = s.tasks.Workflow(ctx, a, team, pid, task, true, func(w *tasks.Workflow) error {
		if e := w.RequireSubmit(false); e != nil {
			return e
		}
		data, media, digest, e := validateFile(name, body)
		if e != nil {
			return e
		}
		prior, e := scanAttachment(w.Tx.QueryRow(ctx, `SELECT `+attachmentColumns+` FROM submission_attachments WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND uploader_id=$4 AND client_request_id=$5`, team, pid, task, a.UserID, key))
		if e == nil {
			if prior.FileName != name || prior.MediaType != media || prior.ByteSize != int64(len(data)) || prior.SHA256 != digest || prior.Status == "expired" || prior.RemovedAt != nil {
				return httpx.Conflict("upload retry key already used or expired")
			}
			out = prior
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e = w.RequireSubmit(true); e != nil {
			return e
		}
		var used int64
		if e = w.Tx.QueryRow(ctx, `SELECT COALESCE(sum(byte_size),0) FROM submission_attachments WHERE team_id=$1 AND removed_at IS NULL`, team).Scan(&used); e != nil {
			return e
		}
		if used+int64(len(data)) > TeamQuotaBytes {
			return httpx.Conflict("team attachment quota of 512 MiB exceeded")
		}
		createdKey, e = s.store.create(team, data)
		if e != nil {
			return httpx.Unavailable("private attachment storage is unavailable")
		}
		out = Attachment{ID: uuid.New(), TeamID: team, ProjectID: pid, TaskID: task, UploaderID: a.UserID, FileName: name, MediaType: media, ByteSize: int64(len(data)), SHA256: digest, Status: "staged", storageKey: createdKey}
		e = w.Tx.QueryRow(ctx, `INSERT INTO submission_attachments(id,team_id,project_id,task_id,uploader_id,client_request_id,storage_key,file_name,media_type,byte_size,sha256,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,clock_timestamp()+interval '24 hours') RETURNING created_at,expires_at`, out.ID, team, pid, task, a.UserID, key, createdKey, name, media, out.ByteSize, digest).Scan(&out.CreatedAt, &out.ExpiresAt)
		if e != nil {
			return e
		}
		return s.projects.Event(ctx, w.Tx, a, team, out.ID, "attachment.staged", "submission_attachment", map[string]any{"project_id": pid, "task_id": task, "byte_size": out.ByteSize, "sha256": digest})
	})
	if err != nil && createdKey != "" {
		// A commit response can be lost. Delete only after a fresh query confirms
		// no committed metadata. Unknown results are recovered by the orphan sweep.
		probe, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var exists bool
		e := s.repo.pool.QueryRow(probe, `SELECT EXISTS(SELECT 1 FROM submission_attachments WHERE storage_key=$1)`, createdKey).Scan(&exists)
		if e == nil && !exists {
			_ = s.store.remove(createdKey)
		}
	}
	return
}
func (s *Service) Discard(ctx context.Context, a reqctx.Identity, team, pid, task, id uuid.UUID) (out Attachment, err error) {
	err = s.tasks.Workflow(ctx, a, team, pid, task, true, func(w *tasks.Workflow) error {
		if e := w.RequireSubmit(false); e != nil {
			return e
		}
		var e error
		out, e = scanAttachment(w.Tx.QueryRow(ctx, `SELECT `+attachmentColumns+` FROM submission_attachments WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND id=$4 AND uploader_id=$5 FOR UPDATE`, team, pid, task, id, a.UserID))
		if errors.Is(e, pgx.ErrNoRows) {
			return projects.NotFound()
		}
		if e != nil {
			return e
		}
		if out.Status != "staged" {
			return httpx.Conflict("only staged attachments may be discarded")
		}
		if _, e = w.Tx.Exec(ctx, `UPDATE submission_attachments SET status='expired' WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND id=$4`, team, pid, task, id); e != nil {
			return e
		}
		out.Status = "expired"
		return s.projects.Event(ctx, w.Tx, a, team, id, "attachment.discarded", "submission_attachment", map[string]any{"project_id": pid, "task_id": task})
	})
	return
}
func (s *Service) Download(ctx context.Context, a reqctx.Identity, team, pid, task, version, id uuid.UUID) (out Attachment, file *os.File, err error) {
	err = s.tasks.Workflow(ctx, a, team, pid, task, false, func(w *tasks.Workflow) error {
		if e := w.RequireRead(); e != nil {
			return e
		}
		var e error
		out, e = scanAttachment(w.Tx.QueryRow(ctx, `SELECT `+attachmentColumns+` FROM submission_attachments WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND submission_version_id=$4 AND id=$5 AND status='bound' AND removed_at IS NULL`, team, pid, task, version, id))
		if errors.Is(e, pgx.ErrNoRows) {
			return projects.NotFound()
		}
		if e != nil {
			return e
		}
		if s.store == nil {
			return httpx.Unavailable("private attachment storage is unavailable")
		}
		file, e = s.store.open(out)
		if e != nil {
			return httpx.Unavailable("attachment content is unavailable; its evidence metadata is preserved")
		}
		return nil
	})
	if err != nil && file != nil {
		file.Close()
		file = nil
	}
	return
}
