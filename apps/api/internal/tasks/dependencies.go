package tasks

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

func (s *Service) Weight(ctx context.Context, a reqctx.Identity, teamID, pid, id uuid.UUID, weight, version int, reason string) (out Task, err error) {
	err = s.within(ctx, a, teamID, pid, true, true, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		t := g.tasks[id]
		if t == nil {
			return projects.NotFound()
		}
		if e := s.execution(ctx, tx, a, sc, *t, false); e != nil {
			return e
		}
		if e := projects.CheckVersion(version, t.Version); e != nil {
			return e
		}
		if e := projects.ValidateReason(reason); e != nil {
			return e
		}
		if weight < 1 || weight > 1000000 {
			return httpx.ValidationFailed(map[string]string{"weight": "must be an integer between 1 and 1000000"})
		}
		if !g.leaf(id) || t.Status == Cancelled {
			return httpx.Conflict("only active leaves have independently editable weights")
		}
		if weight == t.Weight {
			return httpx.Conflict("weight is unchanged")
		}
		before, e := g.progress(nil)
		if e != nil {
			return e
		}
		old := *t
		t.Weight = weight
		if e = s.repo.save(ctx, tx, t); e != nil {
			return e
		}
		if e = s.record(ctx, tx, a, g, old, *t, "weight_changed", strings.TrimSpace(reason), before); e != nil {
			return e
		}
		out, e = s.view(ctx, tx, a, sc, g, *t)
		return e
	})
	return
}
func (s *Service) AddDependency(ctx context.Context, a reqctx.Identity, teamID, pid, id, predecessor uuid.UUID, version int) (out Dependency, err error) {
	err = s.within(ctx, a, teamID, pid, true, true, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		t, p := g.tasks[id], g.tasks[predecessor]
		if t == nil || p == nil {
			return projects.NotFound()
		}
		if e := s.execution(ctx, tx, a, sc, *t, false); e != nil {
			return e
		}
		if e := projects.CheckVersion(version, t.Version); e != nil {
			return e
		}
		if e := s.execution(ctx, tx, a, sc, *p, false); e != nil {
			return e
		}
		if id == predecessor {
			return httpx.Conflict("a task cannot depend on itself")
		}
		if !g.leaf(id) || !g.leaf(predecessor) || t.Status != Pending || p.Status == Cancelled {
			return httpx.Conflict("dependencies require a pending leaf and a non-cancelled predecessor leaf")
		}
		for _, d := range g.deps {
			if d.TaskID == id && d.PredecessorID == predecessor {
				return httpx.Conflict("dependency already exists")
			}
		}
		if g.cycle(id, predecessor) {
			return httpx.Conflict("dependency would form a cycle")
		}
		out = Dependency{ID: uuid.New(), TeamID: teamID, ProjectID: pid, TaskID: id, PredecessorID: predecessor, CreatedBy: a.UserID}
		if e := s.repo.insertDependency(ctx, tx, &out); e != nil {
			return e
		}
		if e := s.repo.save(ctx, tx, t); e != nil {
			return e
		}
		return s.projects.Event(ctx, tx, a, teamID, out.ID, "task.dependency_created", "task_dependency", map[string]any{"project_id": pid, "task_id": id, "predecessor_id": predecessor})
	})
	return
}
func (s *Service) IgnoreDependency(ctx context.Context, a reqctx.Identity, teamID, pid, id, dependencyID uuid.UUID, version int, reason string) (out Dependency, err error) {
	err = s.within(ctx, a, teamID, pid, true, true, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		t := g.tasks[id]
		if t == nil {
			return projects.NotFound()
		}
		if e := s.execution(ctx, tx, a, sc, *t, false); e != nil {
			return e
		}
		if e := projects.CheckVersion(version, t.Version); e != nil {
			return e
		}
		if e := projects.ValidateReason(reason); e != nil {
			return e
		}
		if t.Status == Cancelled {
			return httpx.Conflict("cancelled tasks are read-only")
		}
		var d *Dependency
		for i := range g.deps {
			if g.deps[i].ID == dependencyID && g.deps[i].TaskID == id {
				d = &g.deps[i]
				break
			}
		}
		if d == nil {
			return projects.NotFound()
		}
		if d.IgnoredAt != nil {
			return httpx.Conflict("dependency already ignored")
		}
		now := time.Now().UTC()
		d.IgnoredBy = &a.UserID
		d.IgnoredAt = &now
		d.IgnoreReason = strings.TrimSpace(reason)
		if e := s.repo.ignoreDependency(ctx, tx, *d); e != nil {
			return e
		}
		if e := s.repo.save(ctx, tx, t); e != nil {
			return e
		}
		out = *d
		return s.projects.Event(ctx, tx, a, teamID, d.ID, "task.dependency_ignored", "task_dependency", map[string]any{"project_id": pid, "task_id": id, "predecessor_id": d.PredecessorID, "reason": d.IgnoreReason, "ignored_at": now})
	})
	return
}
