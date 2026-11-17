package tasks

import (
	"context"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) record(ctx context.Context, tx pgx.Tx, a reqctx.Identity, g *graph, old, t Task, event, reason string, before Progress) error {
	after, e := g.progress(nil)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO task_progress_history(team_id,project_id,task_id,actor_id,event_type,old_status,new_status,old_progress,new_progress,old_weight,new_weight,reason,before_project_bps,after_project_bps,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, t.TeamID, t.ProjectID, t.ID, a.UserID, event, old.Status, t.Status, old.ActualProgress, t.ActualProgress, old.Weight, t.Weight, reason, before.ProgressBPS, after.ProgressBPS, reqctx.RequestID(ctx))
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO project_progress_snapshots(team_id,project_id,task_id,actor_id,event_type,progress_bps,weighted_sum,total_weight,leaf_count,request_id) VALUES($1,$2,$3,$4,$5,$6,$7::numeric,$8::numeric,$9,$10)`, t.TeamID, t.ProjectID, t.ID, a.UserID, event, after.ProgressBPS, after.WeightedSum, after.TotalWeight, after.LeafCount, reqctx.RequestID(ctx))
	if e != nil {
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
		rows, e := tx.Query(ctx, `SELECT id,task_id,actor_id,event_type,old_status,new_status,old_progress,new_progress,old_weight,new_weight,reason,before_project_bps,after_project_bps,request_id,created_at FROM task_progress_history WHERE team_id=$1 AND project_id=$2 AND task_id=$3 ORDER BY id DESC LIMIT 200`, teamID, pid, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var h History
			if e = rows.Scan(&h.ID, &h.TaskID, &h.ActorID, &h.EventType, &h.OldStatus, &h.NewStatus, &h.OldProgress, &h.NewProgress, &h.OldWeight, &h.NewWeight, &h.Reason, &h.BeforeProjectBPS, &h.AfterProjectBPS, &h.RequestID, &h.CreatedAt); e != nil {
				return e
			}
			out = append(out, h)
		}
		return rows.Err()
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
		rows, e := tx.Query(ctx, `SELECT id,task_id,actor_id,event_type,progress_bps,weighted_sum::text,total_weight::text,leaf_count,request_id,created_at FROM project_progress_snapshots WHERE team_id=$1 AND project_id=$2 ORDER BY id DESC LIMIT 200`, teamID, pid)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var h Snapshot
			if e = rows.Scan(&h.ID, &h.TaskID, &h.ActorID, &h.EventType, &h.Progress.ProgressBPS, &h.Progress.WeightedSum, &h.Progress.TotalWeight, &h.Progress.LeafCount, &h.RequestID, &h.CreatedAt); e != nil {
				return e
			}
			out = append(out, h)
		}
		return rows.Err()
	})
	return
}
