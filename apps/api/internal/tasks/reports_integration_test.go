//go:build integration

package tasks

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

func reportInput(progress, version int) ProgressReportInput {
	return ProgressReportInput{
		ReportedProgress: &progress,
		WorkSummary:      "完成接口联调与错误处理",
		CompletedWork:    "登录接口、错误提示",
		PendingWork:      "安排设计走查",
		Blockers:         "等待联调环境开放",
		EvidenceLinks:    []string{"https://github.com/YEXIAONAN/Nexora/pull/42", "commit 1a2b3c4"},
		ExpectedVersion:  version,
	}
}

func TestProgressReportRequiresAssignedExecutor(t *testing.T) {
	f := setup(t)
	worker := f.user("worker", true)
	outsider := f.user("outsider", false)
	task := f.svcCreateOwned(worker.UserID)
	task = f.start(task, worker)

	_, err := f.svc.ReportProgress(f.ctx, outsider, f.team, f.p.ID, task.ID, reportInput(40, task.Version))
	wantStatus(t, err, 404)

	// The project lead may coordinate, but a normal Team member who is neither
	// owner nor collaborator must not learn about or report on someone else's task.
	other := f.user("other", true)
	_, err = f.svc.ReportProgress(f.ctx, other, f.team, f.p.ID, task.ID, reportInput(40, task.Version))
	wantStatus(t, err, 404)
}

func TestProgressReportBuildsExecutionHistoryWithoutAcceptance(t *testing.T) {
	f := setup(t)
	worker := f.user("worker", true)
	task := f.svcCreateOwned(worker.UserID)
	task = f.start(task, worker)

	updated, err := f.svc.ReportProgress(f.ctx, worker, f.team, f.p.ID, task.ID, reportInput(40, task.Version))
	if err != nil {
		t.Fatal(err)
	}
	if updated.ActualProgress != 40 || updated.Status != InProgress {
		t.Fatalf("report must update execution progress but not status: %+v", updated)
	}
	if updated.EffectiveProgressBPS != 4000 {
		t.Fatalf("effective progress = %d, want 4000", updated.EffectiveProgressBPS)
	}

	reports, err := f.svc.ProgressReports(f.ctx, worker, f.team, f.p.ID, task.ID)
	if err != nil || len(reports) != 1 {
		t.Fatalf("reports = %d, %v", len(reports), err)
	}
	if reports[0].WorkSummary != "完成接口联调与错误处理" || len(reports[0].EvidenceLinks) != 2 {
		t.Fatalf("report content lost: %+v", reports[0])
	}
	if reports[0].PendingWork != "安排设计走查" || reports[0].Blockers != "等待联调环境开放" {
		t.Fatalf("report plan and blockers lost: %+v", reports[0])
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE task_progress_reports SET work_summary='overwritten' WHERE id=$1`, reports[0].ID); err == nil {
		t.Fatal("historical report was overwritten")
	}
	if reports[0].ActorID != worker.UserID {
		t.Fatalf("report actor = %s", reports[0].ActorID)
	}

	// A progress report is not formal acceptance: the task must not move to
	// awaiting review or completed, and no task_reviews row may be fabricated.
	var reviews int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_reviews WHERE task_id=$1`, task.ID).Scan(&reviews); err != nil || reviews != 0 {
		t.Fatalf("unexpected review evidence: %d %v", reviews, err)
	}
	var events int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_progress_history WHERE task_id=$1 AND event_type='progress_reported'`, task.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("history event = %d, %v", events, err)
	}
}

func TestProgressReportValidationAndState(t *testing.T) {
	f := setup(t)
	worker := f.user("worker", true)
	task := f.svcCreateOwned(worker.UserID)

	_, err := f.svc.ReportProgress(f.ctx, worker, f.team, f.p.ID, task.ID, reportInput(10, task.Version))
	wantStatus(t, err, 409)

	task = f.start(task, worker)
	missing := reportInput(10, task.Version)
	missing.WorkSummary = "   "
	_, err = f.svc.ReportProgress(f.ctx, worker, f.team, f.p.ID, task.ID, missing)
	wantStatus(t, err, 422)

	outOfRange := reportInput(100, task.Version)
	_, err = f.svc.ReportProgress(f.ctx, worker, f.team, f.p.ID, task.ID, outOfRange)
	wantStatus(t, err, 422)

	tooMany := reportInput(20, task.Version)
	for i := 0; i < 21; i++ {
		tooMany.EvidenceLinks = append(tooMany.EvidenceLinks, "https://example.com/"+strings.Repeat("a", 4))
	}
	_, err = f.svc.ReportProgress(f.ctx, worker, f.team, f.p.ID, task.ID, tooMany)
	wantStatus(t, err, 422)

	stale := reportInput(30, task.Version)
	stale.ExpectedVersion = task.Version + 5
	_, err = f.svc.ReportProgress(f.ctx, worker, f.team, f.p.ID, task.ID, stale)
	wantStatus(t, err, 409)

	// Repeated reports at the same percentage are still meaningful evidence.
	if _, err := f.svc.ReportProgress(f.ctx, worker, f.team, f.p.ID, task.ID, reportInput(30, task.Version)); err != nil {
		t.Fatal(err)
	}
	task, _ = f.svc.Get(f.ctx, worker, f.team, f.p.ID, task.ID)
	if _, err := f.svc.ReportProgress(f.ctx, worker, f.team, f.p.ID, task.ID, ProgressReportInput{
		ReportedProgress: func() *int { n := 30; return &n }(),
		WorkSummary:      "继续排查边界条件",
		ExpectedVersion:  task.Version,
	}); err != nil {
		t.Fatalf("same percentage with new evidence must be accepted: %v", err)
	}
	reports, err := f.svc.ProgressReports(f.ctx, worker, f.team, f.p.ID, task.ID)
	if err != nil || len(reports) != 2 {
		t.Fatalf("reports = %d, %v", len(reports), err)
	}
}

func (f *fixture) svcCreateOwned(owner uuid.UUID) Task {
	f.t.Helper()
	task, e := f.svc.Create(f.ctx, f.a, f.team, f.p.ID, CreateInput{TaskFields: TaskFields{Title: "Report task", OwnerID: owner}, Weight: 1})
	if e != nil {
		f.t.Fatal(e)
	}
	return task
}

func (f *fixture) start(task Task, actor reqctx.Identity) Task {
	f.t.Helper()
	next, e := f.svc.Transition(f.ctx, actor, f.team, f.p.ID, task.ID, InProgress, "", task.Version)
	if e != nil {
		f.t.Fatal(e)
	}
	return next
}
