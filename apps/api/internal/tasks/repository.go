package tasks

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

const taskColumns = `id,team_id,project_id,parent_id,depth,title,description,owner_id,priority,planned_start_at,planned_end_at,status,actual_progress,weight,cancellation_reason,version,created_by,created_at,updated_at,current_submission_version_id,accepted_review_id`

func scanTask(row pgx.Row) (t Task, err error) {
	t.CollaboratorIDs = []uuid.UUID{}
	err = row.Scan(&t.ID, &t.TeamID, &t.ProjectID, &t.ParentID, &t.Depth, &t.Title, &t.Description, &t.OwnerID, &t.Priority, &t.PlannedStartAt, &t.PlannedEndAt, &t.Status, &t.ActualProgress, &t.Weight, &t.CancellationReason, &t.Version, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &t.CurrentSubmissionVersionID, &t.AcceptedReviewID)
	return
}
func (r *Repository) load(ctx context.Context, tx pgx.Tx, teamID, projectID uuid.UUID) (*graph, error) {
	g := &graph{tasks: map[uuid.UUID]*Task{}, order: []uuid.UUID{}, deps: []Dependency{}, executed: map[uuid.UUID]bool{}}
	rows, err := tx.Query(ctx, `SELECT `+taskColumns+` FROM tasks WHERE team_id=$1 AND project_id=$2 ORDER BY created_at,id`, teamID, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		t, e := scanTask(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		g.tasks[t.ID] = &t
		g.order = append(g.order, t.ID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, `SELECT task_id,user_id FROM task_collaborators WHERE team_id=$1 AND project_id=$2 ORDER BY user_id`, teamID, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t, u uuid.UUID
		if err = rows.Scan(&t, &u); err != nil {
			rows.Close()
			return nil, err
		}
		g.tasks[t].CollaboratorIDs = append(g.tasks[t].CollaboratorIDs, u)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, `SELECT id,team_id,project_id,task_id,predecessor_id,created_by,created_at,ignored_by,ignored_at,ignore_reason FROM task_dependencies WHERE team_id=$1 AND project_id=$2 ORDER BY created_at,id`, teamID, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d Dependency
		if err = rows.Scan(&d.ID, &d.TeamID, &d.ProjectID, &d.TaskID, &d.PredecessorID, &d.CreatedBy, &d.CreatedAt, &d.IgnoredBy, &d.IgnoredAt, &d.IgnoreReason); err != nil {
			rows.Close()
			return nil, err
		}
		g.deps = append(g.deps, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, `SELECT DISTINCT task_id FROM task_progress_history WHERE team_id=$1 AND project_id=$2 AND event_type IN ('started','progress','cancelled')`, teamID, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		g.executed[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, `SELECT t.id,v.author_id,e.current_version_id,e.reviewed_version_id,e.review_id,e.decision FROM tasks t LEFT JOIN task_submission_versions v ON v.team_id=t.team_id AND v.project_id=t.project_id AND v.task_id=t.id AND v.id=t.current_submission_version_id LEFT JOIN task_acceptance_evidence e ON e.team_id=t.team_id AND e.project_id=t.project_id AND e.task_id=t.id WHERE t.team_id=$1 AND t.project_id=$2`, teamID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var author, current, reviewed, review *uuid.UUID
		var decision *string
		if err = rows.Scan(&id, &author, &current, &reviewed, &review, &decision); err != nil {
			return nil, err
		}
		t := g.tasks[id]
		t.submissionAuthor = author
		if current != nil && reviewed != nil && review != nil && decision != nil {
			t.acceptance = Acceptance{*current, *reviewed, *review, *decision}
		}
	}
	return g, rows.Err()
}
func (r *Repository) insert(ctx context.Context, tx pgx.Tx, t *Task) error {
	return tx.QueryRow(ctx, `INSERT INTO tasks(id,team_id,project_id,parent_id,depth,title,description,owner_id,priority,planned_start_at,planned_end_at,weight,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING version,status,created_at,updated_at`, t.ID, t.TeamID, t.ProjectID, t.ParentID, t.Depth, t.Title, t.Description, t.OwnerID, t.Priority, t.PlannedStartAt, t.PlannedEndAt, t.Weight, t.CreatedBy).Scan(&t.Version, &t.Status, &t.CreatedAt, &t.UpdatedAt)
}
func (r *Repository) save(ctx context.Context, tx pgx.Tx, t *Task) error {
	return tx.QueryRow(ctx, `UPDATE tasks SET parent_id=$4,depth=$5,title=$6,description=$7,owner_id=$8,priority=$9,planned_start_at=$10,planned_end_at=$11,status=$12,actual_progress=$13,weight=$14,cancellation_reason=$15,current_submission_version_id=$16,accepted_review_id=$17,version=version+1,updated_at=clock_timestamp() WHERE team_id=$1 AND project_id=$2 AND id=$3 RETURNING version,updated_at`, t.TeamID, t.ProjectID, t.ID, t.ParentID, t.Depth, t.Title, t.Description, t.OwnerID, t.Priority, t.PlannedStartAt, t.PlannedEndAt, t.Status, t.ActualProgress, t.Weight, t.CancellationReason, t.CurrentSubmissionVersionID, t.AcceptedReviewID).Scan(&t.Version, &t.UpdatedAt)
}
func (r *Repository) collaborators(ctx context.Context, tx pgx.Tx, t Task) error {
	if _, err := tx.Exec(ctx, `DELETE FROM task_collaborators WHERE team_id=$1 AND project_id=$2 AND task_id=$3`, t.TeamID, t.ProjectID, t.ID); err != nil {
		return err
	}
	for _, id := range t.CollaboratorIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO task_collaborators(team_id,project_id,task_id,user_id) VALUES($1,$2,$3,$4)`, t.TeamID, t.ProjectID, t.ID, id); err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) insertDependency(ctx context.Context, tx pgx.Tx, d *Dependency) error {
	return tx.QueryRow(ctx, `INSERT INTO task_dependencies(id,team_id,project_id,task_id,predecessor_id,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, d.ID, d.TeamID, d.ProjectID, d.TaskID, d.PredecessorID, d.CreatedBy).Scan(&d.CreatedAt)
}
func (r *Repository) ignoreDependency(ctx context.Context, tx pgx.Tx, d Dependency) error {
	_, e := tx.Exec(ctx, `UPDATE task_dependencies SET ignored_by=$4,ignored_at=$5,ignore_reason=$6 WHERE team_id=$1 AND project_id=$2 AND id=$3`, d.TeamID, d.ProjectID, d.ID, d.IgnoredBy, d.IgnoredAt, d.IgnoreReason)
	return e
}
