package tasks

import (
	"context"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ReviewAccess supplies live, explicit project grants without a module cycle.
type ReviewAccess interface {
	CanReview(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
}

func (s *Service) SetReviewAccess(r ReviewAccess) { s.reviews = r }
func (s *Service) canReview(ctx context.Context, tx pgx.Tx, team, pid, user uuid.UUID) (bool, error) {
	if s.reviews == nil {
		return false, nil
	}
	return s.reviews.CanReview(ctx, tx, team, pid, user)
}

// Workflow keeps all task mutations, evidence, snapshots and audit under the
// same team-first lock. Only this module writes task state.
type Workflow struct {
	Tx    pgx.Tx
	Scope projects.Scope
	Task  *Task
	ctx   context.Context
	actor reqctx.Identity
	svc   *Service
	graph *graph
}

func (s *Service) Workflow(ctx context.Context, a reqctx.Identity, team, pid, id uuid.UUID, write bool, fn func(*Workflow) error) error {
	return s.within(ctx, a, team, pid, write, false, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		t := g.tasks[id]
		if t == nil {
			return projects.NotFound()
		}
		return fn(&Workflow{tx, sc, t, ctx, a, s, g})
	})
}
func (w *Workflow) RequireRead() error {
	ok, e := w.svc.readable(w.ctx, w.Tx, w.actor, w.Scope, *w.Task)
	if e != nil {
		return e
	}
	if !ok {
		return projects.NotFound()
	}
	return nil
}

// RequireSubmit deliberately requires assignment even for the project lead.
// state=false is used only to authorize an idempotent replay or staged discard.
func (w *Workflow) RequireSubmit(state bool) error {
	t := w.Task
	ok, e := w.svc.projects.Qualified(w.ctx, w.Tx, t.TeamID, t.ProjectID, w.actor.UserID)
	if e != nil {
		return e
	}
	if !ok || !assigned(*t, w.actor.UserID) {
		return projects.Forbidden()
	}
	if !state {
		return nil
	}
	if !w.graph.leaf(t.ID) || w.graph.blocked(t.ID) || (t.Status != InProgress && t.Status != Rejected) {
		return httpx.Conflict("submission requires an unblocked in-progress or rejected leaf")
	}
	owner, e := w.svc.projects.Qualified(w.ctx, w.Tx, t.TeamID, t.ProjectID, t.OwnerID)
	if e != nil {
		return e
	}
	if !owner {
		return httpx.Conflict("reassign inactive task owner before submission")
	}
	return nil
}
func (w *Workflow) SubmitVersion(id uuid.UUID) error {
	if e := w.RequireSubmit(true); e != nil {
		return e
	}
	before, e := w.graph.progress(nil)
	if e != nil {
		return e
	}
	old := *w.Task
	w.Task.Status = AwaitingReview
	w.Task.CurrentSubmissionVersionID = &id
	w.Task.AcceptedReviewID = nil
	w.Task.submissionAuthor = &w.actor.UserID
	w.Task.acceptance = Acceptance{}
	if e = w.svc.repo.save(w.ctx, w.Tx, w.Task); e != nil {
		return e
	}
	return w.svc.record(w.ctx, w.Tx, w.actor, w.graph, old, *w.Task, "submitted", "new immutable submission version; awaiting review is 80%", before)
}
func (w *Workflow) ApplyReview(version, review uuid.UUID, decision, comment string) error {
	t := w.Task
	if t.Status != AwaitingReview || t.CurrentSubmissionVersionID == nil || *t.CurrentSubmissionVersionID != version || !w.graph.leaf(t.ID) {
		return httpx.Conflict("review must target the current awaiting submission version")
	}
	before, e := w.graph.progress(nil)
	if e != nil {
		return e
	}
	old := *t
	event := "review_rejected"
	t.Status = Rejected
	t.AcceptedReviewID = nil
	t.acceptance = Acceptance{}
	if decision == "approve" {
		t.Status = Completed
		t.AcceptedReviewID = &review
		t.acceptance = Acceptance{version, version, review, "approve"}
		event = "review_approved"
	}
	if e = w.svc.repo.save(w.ctx, w.Tx, t); e != nil {
		return e
	}
	return w.svc.record(w.ctx, w.Tx, w.actor, w.graph, old, *t, event, comment, before)
}
