package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ProgressReport is an append-only member report. It records execution
// evidence and the reported percentage; it never represents formal acceptance.
type ProgressReport struct {
	ID               uuid.UUID `json:"id"`
	TeamID           uuid.UUID `json:"team_id"`
	ProjectID        uuid.UUID `json:"project_id"`
	TaskID           uuid.UUID `json:"task_id"`
	ActorID          uuid.UUID `json:"actor_id"`
	ActorDisplayName string    `json:"actor_display_name"`
	ActorUsername    string    `json:"actor_username"`
	ReportedProgress int       `json:"reported_progress"`
	WorkSummary      string    `json:"work_summary"`
	CompletedWork    string    `json:"completed_work"`
	PendingWork      string    `json:"pending_work"`
	Blockers         string    `json:"blockers"`
	EvidenceLinks    []string  `json:"evidence_links"`
	CreatedAt        time.Time `json:"created_at"`
}

// ProgressReportInput is the member-facing payload. ReportedProgress is a
// pointer so a missing field is a validation error, not a silent zero.
type ProgressReportInput struct {
	ReportedProgress *int     `json:"reported_progress"`
	WorkSummary      string   `json:"work_summary"`
	CompletedWork    string   `json:"completed_work"`
	PendingWork      string   `json:"pending_work"`
	Blockers         string   `json:"blockers"`
	EvidenceLinks    []string `json:"evidence_links"`
	ExpectedVersion  int      `json:"expected_version"`
}

const (
	maxReportTextRunes  = 4000
	maxEvidenceLinks    = 20
	maxEvidenceLinkRune = 500
)

func cleanEvidenceLinks(links []string) ([]string, error) {
	cleaned := make([]string, 0, len(links))
	for _, raw := range links {
		link := strings.TrimSpace(raw)
		if link == "" {
			continue
		}
		if utf8.RuneCountInString(link) > maxEvidenceLinkRune {
			return nil, httpx.ValidationFailed(map[string]string{"evidence_links": "each reference must be at most 500 characters"})
		}
		for _, r := range link {
			if unicode.IsControl(r) {
				return nil, httpx.ValidationFailed(map[string]string{"evidence_links": "references must not contain control characters"})
			}
		}
		cleaned = append(cleaned, link)
	}
	if len(cleaned) > maxEvidenceLinks {
		return nil, httpx.ValidationFailed(map[string]string{"evidence_links": "at most 20 references are allowed"})
	}
	return cleaned, nil
}

func (in *ProgressReportInput) normalize() ([]string, error) {
	if in.ReportedProgress == nil {
		return nil, httpx.ValidationFailed(map[string]string{"reported_progress": "is required"})
	}
	if *in.ReportedProgress < 0 || *in.ReportedProgress > 99 {
		return nil, httpx.ValidationFailed(map[string]string{"reported_progress": "must be an integer between 0 and 99"})
	}
	in.WorkSummary = strings.TrimSpace(in.WorkSummary)
	in.CompletedWork = strings.TrimSpace(in.CompletedWork)
	in.PendingWork = strings.TrimSpace(in.PendingWork)
	in.Blockers = strings.TrimSpace(in.Blockers)
	fields := map[string]string{}
	if n := utf8.RuneCountInString(in.WorkSummary); n < 1 || n > maxReportTextRunes {
		fields["work_summary"] = "must be between 1 and 4000 characters"
	}
	if utf8.RuneCountInString(in.CompletedWork) > maxReportTextRunes {
		fields["completed_work"] = "must be at most 4000 characters"
	}
	if utf8.RuneCountInString(in.PendingWork) > maxReportTextRunes {
		fields["pending_work"] = "must be at most 4000 characters"
	}
	if utf8.RuneCountInString(in.Blockers) > maxReportTextRunes {
		fields["blockers"] = "must be at most 4000 characters"
	}
	if len(fields) > 0 {
		return nil, httpx.ValidationFailed(fields)
	}
	links, err := cleanEvidenceLinks(in.EvidenceLinks)
	if err != nil {
		return nil, err
	}
	in.EvidenceLinks = links
	return links, nil
}

// ReportProgress records a structured report and updates the execution
// percentage in the same transaction. It deliberately does not move the task
// to awaiting_review: only the delivery submission flow can request acceptance.
func (s *Service) ReportProgress(ctx context.Context, a reqctx.Identity, teamID, pid, id uuid.UUID, in ProgressReportInput) (out Task, err error) {
	links, err := in.normalize()
	if err != nil {
		return Task{}, err
	}
	err = s.within(ctx, a, teamID, pid, true, false, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
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
		if e := s.execution(ctx, tx, a, sc, *t, false); e != nil {
			return e
		}
		if e := projects.CheckVersion(in.ExpectedVersion, t.Version); e != nil {
			return e
		}
		if !g.leaf(id) || t.Status != InProgress || g.blocked(id) {
			return httpx.Conflict("progress reports require an unblocked in-progress leaf")
		}
		before, e := g.progress(nil)
		if e != nil {
			return e
		}
		old := *t
		t.ActualProgress = *in.ReportedProgress
		if e = s.repo.save(ctx, tx, t); e != nil {
			return e
		}
		if e = s.repo.insertReport(ctx, tx, ProgressReport{
			ID:               uuid.New(),
			TeamID:           teamID,
			ProjectID:        pid,
			TaskID:           id,
			ActorID:          a.UserID,
			ReportedProgress: *in.ReportedProgress,
			WorkSummary:      in.WorkSummary,
			CompletedWork:    in.CompletedWork,
			PendingWork:      in.PendingWork,
			Blockers:         in.Blockers,
			EvidenceLinks:    links,
		}); e != nil {
			return e
		}
		if e = s.record(ctx, tx, a, g, old, *t, "progress_reported", reportReason(in.WorkSummary), before); e != nil {
			return e
		}
		out, e = s.view(ctx, tx, a, sc, g, *t)
		return e
	})
	return out, err
}

func reportReason(summary string) string {
	const limit = 1000
	if utf8.RuneCountInString(summary) <= limit {
		return summary
	}
	runes := []rune(summary)
	return string(runes[:limit])
}

// ProgressReports lists reports visible with the same scope as the task.
func (s *Service) ProgressReports(ctx context.Context, a reqctx.Identity, teamID, pid, id uuid.UUID) (out []ProgressReport, err error) {
	out = []ProgressReport{}
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
		out, e = s.repo.listReports(ctx, tx, teamID, pid, id)
		return e
	})
	return out, err
}

func (r *Repository) insertReport(ctx context.Context, tx pgx.Tx, report ProgressReport) error {
	links, err := json.Marshal(report.EvidenceLinks)
	if err != nil {
		return fmt.Errorf("encode evidence links: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO task_progress_reports
			(id, team_id, project_id, task_id, actor_id, reported_progress, work_summary, completed_work, pending_work, evidence_links, blockers)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)`,
		report.ID, report.TeamID, report.ProjectID, report.TaskID, report.ActorID,
		report.ReportedProgress, report.WorkSummary, report.CompletedWork, report.PendingWork, links, report.Blockers)
	if err != nil {
		return fmt.Errorf("insert progress report: %w", err)
	}
	return nil
}

func (r *Repository) listReports(ctx context.Context, tx pgx.Tx, teamID, pid, taskID uuid.UUID) ([]ProgressReport, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.team_id, r.project_id, r.task_id, r.actor_id,
		       u.display_name, u.username, r.reported_progress,
		       r.work_summary, r.completed_work, r.pending_work, r.evidence_links, r.created_at, r.blockers
		FROM task_progress_reports r
		JOIN users u ON u.id = r.actor_id
		WHERE r.team_id=$1 AND r.project_id=$2 AND r.task_id=$3
		ORDER BY r.created_at DESC, r.id DESC`, teamID, pid, taskID)
	if err != nil {
		return nil, fmt.Errorf("list progress reports: %w", err)
	}
	defer rows.Close()
	out := []ProgressReport{}
	for rows.Next() {
		var report ProgressReport
		var links []byte
		if err := rows.Scan(&report.ID, &report.TeamID, &report.ProjectID, &report.TaskID, &report.ActorID,
			&report.ActorDisplayName, &report.ActorUsername, &report.ReportedProgress,
			&report.WorkSummary, &report.CompletedWork, &report.PendingWork, &links, &report.CreatedAt, &report.Blockers); err != nil {
			return nil, fmt.Errorf("scan progress report: %w", err)
		}
		report.EvidenceLinks = []string{}
		if len(links) > 0 {
			if err := json.Unmarshal(links, &report.EvidenceLinks); err != nil {
				return nil, fmt.Errorf("decode evidence links: %w", err)
			}
		}
		out = append(out, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list progress reports: %w", err)
	}
	return out, nil
}
