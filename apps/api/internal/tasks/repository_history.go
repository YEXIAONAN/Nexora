package tasks

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) appendRecords(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, requestID string, old, t Task, event, reason string, before, after Progress) error {
	_, e := tx.Exec(ctx, `INSERT INTO task_progress_history(team_id,project_id,task_id,actor_id,event_type,old_status,new_status,old_progress,new_progress,old_weight,new_weight,reason,before_project_bps,after_project_bps,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, t.TeamID, t.ProjectID, t.ID, actorID, event, old.Status, t.Status, old.ActualProgress, t.ActualProgress, old.Weight, t.Weight, reason, before.ProgressBPS, after.ProgressBPS, requestID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO project_progress_snapshots(team_id,project_id,task_id,actor_id,event_type,progress_bps,weighted_sum,total_weight,leaf_count,request_id) VALUES($1,$2,$3,$4,$5,$6,$7::numeric,$8::numeric,$9,$10)`, t.TeamID, t.ProjectID, t.ID, actorID, event, after.ProgressBPS, after.WeightedSum, after.TotalWeight, after.LeafCount, requestID)
	return e
}
func (r *Repository) listHistory(ctx context.Context, tx pgx.Tx, teamID, pid, id uuid.UUID) ([]History, error) {
	rows, e := tx.Query(ctx, `SELECT id,task_id,actor_id,event_type,old_status,new_status,old_progress,new_progress,old_weight,new_weight,reason,before_project_bps,after_project_bps,request_id,created_at FROM task_progress_history WHERE team_id=$1 AND project_id=$2 AND task_id=$3 ORDER BY id DESC LIMIT 200`, teamID, pid, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []History{}
	for rows.Next() {
		var h History
		if e = rows.Scan(&h.ID, &h.TaskID, &h.ActorID, &h.EventType, &h.OldStatus, &h.NewStatus, &h.OldProgress, &h.NewProgress, &h.OldWeight, &h.NewWeight, &h.Reason, &h.BeforeProjectBPS, &h.AfterProjectBPS, &h.RequestID, &h.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
func (r *Repository) listSnapshots(ctx context.Context, tx pgx.Tx, teamID, pid uuid.UUID) ([]Snapshot, error) {
	rows, e := tx.Query(ctx, `SELECT id,task_id,actor_id,event_type,progress_bps,weighted_sum::text,total_weight::text,leaf_count,request_id,created_at FROM project_progress_snapshots WHERE team_id=$1 AND project_id=$2 ORDER BY id DESC LIMIT 200`, teamID, pid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Snapshot{}
	for rows.Next() {
		var h Snapshot
		if e = rows.Scan(&h.ID, &h.TaskID, &h.ActorID, &h.EventType, &h.Progress.ProgressBPS, &h.Progress.WeightedSum, &h.Progress.TotalWeight, &h.Progress.LeafCount, &h.RequestID, &h.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
