package projects

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const projectColumns = `id,team_id,name,description,lead_id,status,planned_start_at,planned_end_at,archived_at,version,created_by,created_at,updated_at`

func scanProject(row pgx.Row) (p Project, err error) {
	err = row.Scan(&p.ID, &p.TeamID, &p.Name, &p.Description, &p.LeadID, &p.Status, &p.PlannedStartAt, &p.PlannedEndAt, &p.ArchivedAt, &p.Version, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	return
}
func (r *Repository) get(ctx context.Context, tx pgx.Tx, teamID, id uuid.UUID) (Project, error) {
	return scanProject(tx.QueryRow(ctx, `SELECT `+projectColumns+` FROM projects WHERE team_id=$1 AND id=$2`, teamID, id))
}
func (r *Repository) list(ctx context.Context, tx pgx.Tx, teamID uuid.UUID) ([]Project, error) {
	rows, err := tx.Query(ctx, `SELECT `+projectColumns+` FROM projects WHERE team_id=$1 ORDER BY created_at DESC,id`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, e := scanProject(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (r *Repository) insert(ctx context.Context, tx pgx.Tx, p *Project) error {
	return tx.QueryRow(ctx, `INSERT INTO projects(id,team_id,name,description,lead_id,planned_start_at,planned_end_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING status,version,created_at,updated_at`, p.ID, p.TeamID, p.Name, p.Description, p.LeadID, p.PlannedStartAt, p.PlannedEndAt, p.CreatedBy).Scan(&p.Status, &p.Version, &p.CreatedAt, &p.UpdatedAt)
}
func (r *Repository) save(ctx context.Context, tx pgx.Tx, p *Project) error {
	return tx.QueryRow(ctx, `UPDATE projects SET name=$3,description=$4,lead_id=$5,status=$6,planned_start_at=$7,planned_end_at=$8,archived_at=$9,version=version+1,updated_at=now() WHERE team_id=$1 AND id=$2 RETURNING version,updated_at`, p.TeamID, p.ID, p.Name, p.Description, p.LeadID, p.Status, p.PlannedStartAt, p.PlannedEndAt, p.ArchivedAt).Scan(&p.Version, &p.UpdatedAt)
}

// Qualified is a live Team + project qualification check within the caller's lock.
func (r *Repository) Qualified(ctx context.Context, tx pgx.Tx, teamID, projectID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members p JOIN team_members m ON m.team_id=p.team_id AND m.user_id=p.user_id WHERE p.team_id=$1 AND p.project_id=$2 AND p.user_id=$3 AND p.status='active' AND m.status='active')`, teamID, projectID, userID).Scan(&ok)
	return ok, err
}
func (r *Repository) addMember(ctx context.Context, tx pgx.Tx, teamID, projectID, userID uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO project_members(team_id,project_id,user_id) VALUES($1,$2,$3) ON CONFLICT(team_id,project_id,user_id) DO UPDATE SET status='active',updated_at=now()`, teamID, projectID, userID)
	return err
}
func (r *Repository) members(ctx context.Context, tx pgx.Tx, p Project) ([]Member, error) {
	rows, err := tx.Query(ctx, `SELECT pm.user_id,u.username,u.display_name,pm.status='active' AND tm.status='active' FROM project_members pm JOIN users u ON u.id=pm.user_id JOIN team_members tm ON tm.team_id=pm.team_id AND tm.user_id=pm.user_id WHERE pm.team_id=$1 AND pm.project_id=$2 ORDER BY pm.created_at,pm.user_id`, p.TeamID, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err = rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.Active); err != nil {
			return nil, err
		}
		m.Role = "participant"
		if m.UserID == p.LeadID {
			m.Role = "lead"
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) removeMember(ctx context.Context, tx pgx.Tx, teamID, projectID, userID uuid.UUID) error {
	tag, err := tx.Exec(ctx, `UPDATE project_members SET status='removed',updated_at=now() WHERE team_id=$1 AND project_id=$2 AND user_id=$3 AND status='active'`, teamID, projectID, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

const milestoneColumns = `id,team_id,project_id,name,description,planned_at,version,created_at,updated_at`

func scanMilestone(row pgx.Row) (m Milestone, err error) {
	err = row.Scan(&m.ID, &m.TeamID, &m.ProjectID, &m.Name, &m.Description, &m.PlannedAt, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	return
}
func (r *Repository) milestones(ctx context.Context, tx pgx.Tx, teamID, projectID uuid.UUID) ([]Milestone, error) {
	rows, err := tx.Query(ctx, `SELECT `+milestoneColumns+` FROM project_milestones WHERE team_id=$1 AND project_id=$2 ORDER BY planned_at NULLS LAST,created_at,id`, teamID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Milestone{}
	for rows.Next() {
		m, e := scanMilestone(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (r *Repository) milestone(ctx context.Context, tx pgx.Tx, teamID, projectID, id uuid.UUID) (Milestone, error) {
	return scanMilestone(tx.QueryRow(ctx, `SELECT `+milestoneColumns+` FROM project_milestones WHERE team_id=$1 AND project_id=$2 AND id=$3`, teamID, projectID, id))
}
func (r *Repository) saveMilestone(ctx context.Context, tx pgx.Tx, m *Milestone, create bool) error {
	if create {
		return tx.QueryRow(ctx, `INSERT INTO project_milestones(id,team_id,project_id,name,description,planned_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING version,created_at,updated_at`, m.ID, m.TeamID, m.ProjectID, m.Name, m.Description, m.PlannedAt).Scan(&m.Version, &m.CreatedAt, &m.UpdatedAt)
	}
	return tx.QueryRow(ctx, `UPDATE project_milestones SET name=$4,description=$5,planned_at=$6,version=version+1,updated_at=now() WHERE team_id=$1 AND project_id=$2 AND id=$3 RETURNING version,updated_at`, m.TeamID, m.ProjectID, m.ID, m.Name, m.Description, m.PlannedAt).Scan(&m.Version, &m.UpdatedAt)
}
func missing(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
