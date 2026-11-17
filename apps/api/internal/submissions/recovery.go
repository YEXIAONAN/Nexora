package submissions

import (
	"context"
	"errors"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/google/uuid"
	"os"
	"time"
)

// Sweep expires abandoned staged uploads. Deleted bytes stop consuming quota
// only after successful physical removal. Bound evidence is never collected.
func (s *Service) Sweep(ctx context.Context, now time.Time) error {
	if s.store == nil {
		return nil
	}
	rows, e := s.repo.pool.Query(ctx, `SELECT DISTINCT team_id FROM submission_attachments`)
	if e != nil {
		return e
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	ar := audit.NewRepository(s.repo.pool)
	for _, team := range ids {
		tx, e := s.repo.pool.Begin(ctx)
		if e != nil {
			return e
		}
		e = func() error {
			if _, e := tx.Exec(ctx, `SELECT id FROM teams WHERE id=$1 FOR UPDATE`, team); e != nil {
				return e
			}
			rows, e := tx.Query(ctx, `UPDATE submission_attachments SET status='expired' WHERE team_id=$1 AND status='staged' AND expires_at<=$2 RETURNING id`, team, now)
			if e != nil {
				return e
			}
			expired := []uuid.UUID{}
			for rows.Next() {
				var id uuid.UUID
				if e = rows.Scan(&id); e != nil {
					rows.Close()
					return e
				}
				expired = append(expired, id)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			for _, id := range expired {
				if e = ar.Insert(ctx, tx, audit.Event{Action: "attachment.expired", TargetType: "submission_attachment", TargetID: &id, TeamID: &team, RequestID: "attachment-recovery", Details: map[string]any{"reason": "staged upload TTL elapsed"}}); e != nil {
					return e
				}
			}
			return nil
		}()
		if e == nil {
			e = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if e != nil {
			return e
		}
		// Commit expiration first. Failure here leaves expired, quota-counted data
		// ready for the next sweep rather than a reference to a deleted bound file.
		rows, e = s.repo.pool.Query(ctx, `SELECT id,storage_key FROM submission_attachments WHERE team_id=$1 AND status='expired' AND removed_at IS NULL`, team)
		if e != nil {
			return e
		}
		type entry struct {
			id  uuid.UUID
			key string
		}
		files := []entry{}
		for rows.Next() {
			var f entry
			if e = rows.Scan(&f.id, &f.key); e != nil {
				rows.Close()
				return e
			}
			files = append(files, f)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, f := range files {
			if e = s.store.remove(f.key); e != nil {
				return e
			}
			if _, e = s.repo.pool.Exec(ctx, `UPDATE submission_attachments SET removed_at=clock_timestamp() WHERE team_id=$1 AND id=$2 AND status='expired' AND removed_at IS NULL`, team, f.id); e != nil {
				return e
			}
		}
	}
	return s.sweepOrphans(ctx, now.Add(-24*time.Hour))
}
func (s *Service) sweepOrphans(ctx context.Context, cutoff time.Time) error {
	dir, e := s.store.root.Open(".")
	if e != nil {
		return e
	}
	teams, e := dir.ReadDir(-1)
	dir.Close()
	if e != nil {
		return e
	}
	for _, team := range teams {
		if !team.IsDir() || team.Type()&os.ModeSymlink != 0 {
			continue
		}
		id, e := uuid.Parse(team.Name())
		if e != nil || id.String() != team.Name() {
			continue
		}
		d, e := s.store.root.Open(team.Name())
		if e != nil {
			return e
		}
		files, e := d.ReadDir(-1)
		d.Close()
		if e != nil {
			return e
		}
		for _, file := range files {
			key := team.Name() + "/" + file.Name()
			if !validKey(key) || !file.Type().IsRegular() {
				continue
			}
			info, e := file.Info()
			if e != nil {
				return e
			}
			if !info.ModTime().Before(cutoff) {
				continue
			}
			var exists bool
			if e = s.repo.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM submission_attachments WHERE storage_key=$1)`, key).Scan(&exists); e != nil {
				return e
			}
			if !exists {
				if e = s.store.remove(key); e != nil && !errors.Is(e, os.ErrNotExist) {
					return e
				}
			}
		}
	}
	return nil
}
