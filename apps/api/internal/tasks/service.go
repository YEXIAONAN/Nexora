package tasks

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

type Service struct {
	repo     *Repository
	projects *projects.Service
	reviews  ReviewAccess
}

func NewService(r *Repository, p *projects.Service) *Service { return &Service{repo: r, projects: p} }
func (s *Service) within(ctx context.Context, a reqctx.Identity, teamID, pid uuid.UUID, write, lead bool, fn func(pgx.Tx, projects.Scope, *graph) error) error {
	return s.projects.Within(ctx, a, teamID, pid, write, lead, func(tx pgx.Tx, sc projects.Scope) error {
		g, e := s.repo.load(ctx, tx, teamID, pid)
		if e != nil {
			return e
		}
		return fn(tx, sc, g)
	})
}
func assigned(t Task, id uuid.UUID) bool {
	if t.OwnerID == id {
		return true
	}
	for _, u := range t.CollaboratorIDs {
		if u == id {
			return true
		}
	}
	return false
}
func (s *Service) readable(ctx context.Context, tx pgx.Tx, a reqctx.Identity, sc projects.Scope, t Task) (bool, error) {
	if sc.Lead {
		return true, nil
	}
	ok, e := s.projects.Qualified(ctx, tx, t.TeamID, t.ProjectID, a.UserID)
	if e != nil || !ok {
		return false, e
	}
	if assigned(t, a.UserID) {
		return true, nil
	}
	return s.canReview(ctx, tx, t.TeamID, t.ProjectID, a.UserID)
}
func (s *Service) execution(ctx context.Context, tx pgx.Tx, a reqctx.Identity, sc projects.Scope, t Task, cancel bool) error {
	if !sc.Lead {
		ok, e := s.projects.Qualified(ctx, tx, t.TeamID, t.ProjectID, a.UserID)
		if e != nil {
			return e
		}
		if !ok || !assigned(t, a.UserID) || (cancel && a.UserID != t.OwnerID) {
			return projects.Forbidden()
		}
	}
	ok, e := s.projects.Qualified(ctx, tx, t.TeamID, t.ProjectID, t.OwnerID)
	if e != nil {
		return e
	}
	if !ok {
		return httpx.Conflict("reassign this task to an active project member before execution")
	}
	return nil
}
func (s *Service) validate(ctx context.Context, tx pgx.Tx, teamID, pid uuid.UUID, f *TaskFields) error {
	f.Title = strings.TrimSpace(f.Title)
	fields := map[string]string{}
	if n := utf8.RuneCountInString(f.Title); n < 1 || n > 200 {
		fields["title"] = "must be between 1 and 200 characters"
	}
	if utf8.RuneCountInString(f.Description) > 8000 {
		fields["description"] = "must be at most 8000 characters"
	}
	if f.Priority == "" {
		f.Priority = "normal"
	}
	if f.Priority != "low" && f.Priority != "normal" && f.Priority != "high" && f.Priority != "urgent" {
		fields["priority"] = "must be low, normal, high or urgent"
	}
	if f.PlannedStartAt != nil && f.PlannedEndAt != nil && f.PlannedStartAt.After(*f.PlannedEndAt) {
		fields["planned_end_at"] = "must not precede planned_start_at"
	}
	if f.CollaboratorIDs == nil {
		f.CollaboratorIDs = []uuid.UUID{}
	}
	if len(f.CollaboratorIDs) > 50 {
		fields["collaborator_ids"] = "must contain at most 50 members"
	}
	seen := map[uuid.UUID]bool{f.OwnerID: true}
	for _, id := range f.CollaboratorIDs {
		if seen[id] {
			fields["collaborator_ids"] = "must be unique and exclude the owner"
		}
		seen[id] = true
	}
	if len(fields) > 0 {
		return httpx.ValidationFailed(fields)
	}
	for id := range seen {
		ok, e := s.projects.Qualified(ctx, tx, teamID, pid, id)
		if e != nil {
			return e
		}
		if !ok {
			return httpx.ValidationFailed(map[string]string{"participants": "owner and collaborators require active project and team membership"})
		}
	}
	return nil
}
func (s *Service) view(ctx context.Context, tx pgx.Tx, a reqctx.Identity, sc projects.Scope, g *graph, t Task) (Task, error) {
	t.IsLeaf = g.leaf(t.ID)
	t.Blocked = g.blocked(t.ID)
	p, e := g.progress(&t.ID)
	if e != nil {
		return Task{}, e
	}
	t.EffectiveProgressBPS = p.ProgressBPS
	if !t.IsLeaf {
		t.Status = Pending
		allCancelled := true
		for _, child := range g.tasks {
			if !g.leaf(child.ID) || !g.descendant(child.ID, t.ID) {
				continue
			}
			if child.Status != Cancelled {
				allCancelled = false
			}
			if child.Status == InProgress || child.Status == AwaitingReview || child.Status == Rejected || child.Status == Completed {
				t.Status = InProgress
			}
		}
		if allCancelled {
			t.Status = Cancelled
		} else if p.LeafCount > 0 && p.ProgressBPS == 10000 {
			t.Status = Completed
		}
	}
	allowed, e := s.readable(ctx, tx, a, sc, t)
	if e != nil {
		return Task{}, e
	}
	ownerOK, e := s.projects.Qualified(ctx, tx, t.TeamID, t.ProjectID, t.OwnerID)
	if e != nil {
		return Task{}, e
	}
	qualified, e := s.projects.Qualified(ctx, tx, t.TeamID, t.ProjectID, a.UserID)
	if e != nil {
		return Task{}, e
	}
	executor := allowed && (sc.Lead || (qualified && assigned(t, a.UserID)))
	base := executor && ownerOK && sc.Project.Status == "active" && t.IsLeaf && (t.Status == Pending || t.Status == InProgress || t.Status == Rejected)
	t.CanExecute = base && !t.Blocked
	t.CanCancel = base && (sc.Lead || t.OwnerID == a.UserID)
	t.CanSubmit = qualified && assigned(t, a.UserID) && ownerOK && sc.Project.Status == "active" && t.IsLeaf && !t.Blocked && (t.Status == InProgress || t.Status == Rejected)
	reviewer, e := s.canReview(ctx, tx, t.TeamID, t.ProjectID, a.UserID)
	if e != nil {
		return Task{}, e
	}
	t.CanReview = reviewer && t.IsLeaf && sc.Project.Status == "active" && t.Status == AwaitingReview && t.submissionAuthor != nil && *t.submissionAuthor != a.UserID
	return t, nil
}
func (s *Service) Tree(ctx context.Context, a reqctx.Identity, teamID, pid uuid.UUID) (out Tree, err error) {
	out.Tasks = []Task{}
	out.Dependencies = []Dependency{}
	err = s.within(ctx, a, teamID, pid, false, false, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		visible := map[uuid.UUID]bool{}
		for _, id := range g.order {
			t := *g.tasks[id]
			ok, e := s.readable(ctx, tx, a, sc, t)
			if e != nil {
				return e
			}
			if !ok {
				continue
			}
			v, e := s.view(ctx, tx, a, sc, g, t)
			if e != nil {
				return e
			}
			out.Tasks = append(out.Tasks, v)
			visible[id] = true
		}
		for _, d := range g.deps {
			if sc.Lead || (visible[d.TaskID] && visible[d.PredecessorID]) {
				out.Dependencies = append(out.Dependencies, d)
			}
		}
		var e error
		out.Progress, e = g.progress(nil)
		return e
	})
	return
}
func (s *Service) Get(ctx context.Context, a reqctx.Identity, teamID, pid, id uuid.UUID) (out Task, err error) {
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
		out, e = s.view(ctx, tx, a, sc, g, *t)
		return e
	})
	return
}
func (s *Service) Create(ctx context.Context, a reqctx.Identity, teamID, pid uuid.UUID, in CreateInput) (out Task, err error) {
	err = s.within(ctx, a, teamID, pid, true, true, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		if in.Weight < 1 || in.Weight > 1000000 {
			return httpx.ValidationFailed(map[string]string{"weight": "must be an integer between 1 and 1000000"})
		}
		if e := s.validate(ctx, tx, teamID, pid, &in.TaskFields); e != nil {
			return e
		}
		depth := 1
		if in.ParentID != nil {
			p := g.tasks[*in.ParentID]
			if p == nil {
				return projects.NotFound()
			}
			if e := s.execution(ctx, tx, a, sc, *p, false); e != nil {
				return e
			}
			if p.Status == Cancelled {
				return httpx.Conflict("cannot add a child to a cancelled task")
			}
			depth = p.Depth + 1
			if depth > 3 {
				return httpx.Conflict("tasks support at most three levels")
			}
			if g.leaf(p.ID) && !g.canBecomeParent(p.ID) {
				return httpx.Conflict("a task with execution history or dependencies cannot become a summary node")
			}
		}
		before, e := g.progress(nil)
		if e != nil {
			return e
		}
		out = Task{TaskFields: in.TaskFields, ID: uuid.New(), TeamID: teamID, ProjectID: pid, ParentID: in.ParentID, Depth: depth, Weight: in.Weight, CreatedBy: a.UserID}
		if e = s.repo.insert(ctx, tx, &out); e != nil {
			return e
		}
		if e = s.repo.collaborators(ctx, tx, out); e != nil {
			return e
		}
		g.tasks[out.ID] = &out
		g.nonLeaves = nil
		g.order = append(g.order, out.ID)
		if e = s.record(ctx, tx, a, g, Task{ID: out.ID, Status: Pending}, out, "created", "", before); e != nil {
			return e
		}
		v, e := s.view(ctx, tx, a, sc, g, out)
		out = v
		return e
	})
	return
}
func (s *Service) Edit(ctx context.Context, a reqctx.Identity, teamID, pid, id uuid.UUID, in EditInput) (out Task, err error) {
	err = s.within(ctx, a, teamID, pid, true, true, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		t := g.tasks[id]
		if t == nil {
			return projects.NotFound()
		}
		if e := projects.CheckVersion(in.ExpectedVersion, t.Version); e != nil {
			return e
		}
		if t.Status == Cancelled || t.Status == Completed || t.Status == AwaitingReview {
			return httpx.Conflict("cancelled, completed and awaiting-review tasks are read-only")
		}
		if e := s.validate(ctx, tx, teamID, pid, &in.TaskFields); e != nil {
			return e
		}
		before := *t
		t.TaskFields = in.TaskFields
		if e := s.repo.save(ctx, tx, t); e != nil {
			return e
		}
		if e := s.repo.collaborators(ctx, tx, *t); e != nil {
			return e
		}
		if e := s.projects.Event(ctx, tx, a, teamID, id, "task.edited", "task", map[string]any{"before": before, "after": *t}); e != nil {
			return e
		}
		var e error
		out, e = s.view(ctx, tx, a, sc, g, *t)
		return e
	})
	return
}
func (s *Service) Move(ctx context.Context, a reqctx.Identity, teamID, pid, id uuid.UUID, parent *uuid.UUID, version int) (out Task, err error) {
	err = s.within(ctx, a, teamID, pid, true, true, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		t := g.tasks[id]
		if t == nil {
			return projects.NotFound()
		}
		if e := projects.CheckVersion(version, t.Version); e != nil {
			return e
		}
		if t.Status == Cancelled || t.Status == Completed || t.Status == AwaitingReview {
			return httpx.Conflict("cancelled, completed and awaiting-review tasks are read-only")
		}
		depth := 1
		if parent != nil {
			p := g.tasks[*parent]
			if p == nil {
				return projects.NotFound()
			}
			if p.ID == id || g.descendant(p.ID, id) {
				return httpx.Conflict("a task cannot be moved under itself or a descendant")
			}
			if e := s.execution(ctx, tx, a, sc, *p, false); e != nil {
				return e
			}
			if p.Status == Cancelled {
				return httpx.Conflict("cannot move under a cancelled task")
			}
			if g.leaf(p.ID) && !g.canBecomeParent(p.ID) {
				return httpx.Conflict("target leaf has execution history or dependencies")
			}
			depth = p.Depth + 1
		}
		if (parent == nil && t.ParentID == nil) || (parent != nil && t.ParentID != nil && *parent == *t.ParentID) {
			return httpx.Conflict("task already has this parent")
		}
		delta := depth - t.Depth
		ids := []uuid.UUID{id}
		for child := range g.tasks {
			if g.descendant(child, id) {
				ids = append(ids, child)
			}
		}
		for _, child := range ids {
			if g.tasks[child].Status == Completed || g.tasks[child].Status == AwaitingReview {
				return httpx.Conflict("a subtree with submitted or completed work cannot be moved")
			}
			if g.tasks[child].Depth+delta > 3 {
				return httpx.Conflict("moving this subtree would exceed three levels")
			}
		}
		before, e := g.progress(nil)
		if e != nil {
			return e
		}
		old := *t
		t.ParentID = parent
		g.nonLeaves = nil
		for _, child := range ids {
			node := g.tasks[child]
			if e := s.execution(ctx, tx, a, sc, *node, false); e != nil {
				return e
			}
			node.Depth += delta
			if e = s.repo.save(ctx, tx, node); e != nil {
				return e
			}
		}
		if e = s.record(ctx, tx, a, g, old, *t, "moved", "", before); e != nil {
			return e
		}
		out, e = s.view(ctx, tx, a, sc, g, *t)
		return e
	})
	return
}
func (s *Service) Transition(ctx context.Context, a reqctx.Identity, teamID, pid, id uuid.UUID, to, reason string, version int) (out Task, err error) {
	err = s.within(ctx, a, teamID, pid, true, false, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
		t := g.tasks[id]
		if t == nil {
			return projects.NotFound()
		}
		if e := s.execution(ctx, tx, a, sc, *t, to == Cancelled); e != nil {
			return e
		}
		if e := projects.CheckVersion(version, t.Version); e != nil {
			return e
		}
		if !g.leaf(id) {
			return httpx.Conflict("summary nodes do not use leaf transitions")
		}
		if !transitionAllowed(t.Status, to) {
			return httpx.Conflict("this transition requires Phase 04 business evidence or is not allowed")
		}
		if to == Cancelled || t.Status == Rejected {
			if e := projects.ValidateReason(reason); e != nil {
				return e
			}
		}
		if to != Cancelled && g.blocked(id) {
			return httpx.Conflict("task is blocked by an unfulfilled dependency")
		}
		before, e := g.progress(nil)
		if e != nil {
			return e
		}
		old := *t
		t.Status = to
		event := "started"
		if old.Status == Rejected && to == InProgress {
			event = "correction_started"
		}
		if to == Cancelled {
			t.CancellationReason = strings.TrimSpace(reason)
			event = "cancelled"
		}
		if e = s.repo.save(ctx, tx, t); e != nil {
			return e
		}
		if e = s.record(ctx, tx, a, g, old, *t, event, reason, before); e != nil {
			return e
		}
		out, e = s.view(ctx, tx, a, sc, g, *t)
		return e
	})
	return
}
func (s *Service) Progress(ctx context.Context, a reqctx.Identity, teamID, pid, id uuid.UUID, actual, version int) (out Task, err error) {
	err = s.within(ctx, a, teamID, pid, true, false, func(tx pgx.Tx, sc projects.Scope, g *graph) error {
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
		if actual < 0 || actual > 99 {
			return httpx.ValidationFailed(map[string]string{"actual_progress": "must be an integer between 0 and 99"})
		}
		if !g.leaf(id) || t.Status != InProgress || g.blocked(id) {
			return httpx.Conflict("progress requires an unblocked in-progress leaf")
		}
		if actual == t.ActualProgress {
			return httpx.Conflict("progress is unchanged")
		}
		before, e := g.progress(nil)
		if e != nil {
			return e
		}
		old := *t
		t.ActualProgress = actual
		if e = s.repo.save(ctx, tx, t); e != nil {
			return e
		}
		if e = s.record(ctx, tx, a, g, old, *t, "progress", "", before); e != nil {
			return e
		}
		out, e = s.view(ctx, tx, a, sc, g, *t)
		return e
	})
	return
}
