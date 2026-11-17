package tasks

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

func (s *Service) record(ctx context.Context, tx pgx.Tx, a reqctx.Identity, g *graph, old, t Task, event, reason string, before Progress) error {
	after, e := g.progress(nil)
	if e != nil {
		return e
	}
	if e = s.repo.appendRecords(ctx, tx, a.UserID, reqctx.RequestID(ctx), old, t, event, reason, before, after); e != nil {
		return e
	}
	return s.projects.Event(ctx, tx, a, t.TeamID, t.ID, "task."+event, "task", map[string]any{"project_id": t.ProjectID, "old_status": old.Status, "new_status": t.Status, "old_progress": old.ActualProgress, "new_progress": t.ActualProgress, "old_weight": old.Weight, "new_weight": t.Weight, "old_parent_id": old.ParentID, "new_parent_id": t.ParentID, "reason": reason, "before_project_bps": before.ProgressBPS, "after_project_bps": after.ProgressBPS})
}
func (s *Service) History(ctx context.Context, a reqctx.Identity, teamID, pid, id uuid.UUID) (out []History, err error) {
	out = []History{}
	err = s.within(ctx, a, teamID, pid, false, false, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		t := g.tasks[id]
		if t == nil {
			return projects.NotFound()
		}
		ok, e := s.readable(ctx, tx, a, sc, *t)
		if e != nil {
			return e
		}
		if !ok {
			return projects.NotFound()
		}
		var e2 error
		out, e2 = s.repo.listHistory(ctx, tx, teamID, pid, id)
		return e2
	})
	return
}
func (s *Service) ProjectProgress(ctx context.Context, a reqctx.Identity, teamID, pid uuid.UUID) (out Progress, err error) {
	err = s.within(ctx, a, teamID, pid, false, false, func(tx pgx.Tx, sc projects.Scope, g *graph) error { var e error; out, e = g.progress(nil); return e })
	return
}
func (s *Service) Snapshots(ctx context.Context, a reqctx.Identity, teamID, pid uuid.UUID) (out []Snapshot, err error) {
	out = []Snapshot{}
	err = s.projects.Within(ctx, a, teamID, pid, false, false, func(tx pgx.Tx, sc projects.Scope) error {
		var e error
		out, e = s.repo.listSnapshots(ctx, tx, teamID, pid)
		return e
	})
	return
}
