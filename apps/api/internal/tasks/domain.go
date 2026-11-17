// Package tasks owns task trees, execution, dependencies and progress history.
package tasks

import (
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
)

const (
	Pending        = "pending"
	InProgress     = "in_progress"
	AwaitingReview = "awaiting_review"
	Completed      = "completed"
	Rejected       = "rejected"
	Cancelled      = "cancelled"
)

type TaskFields struct {
	Title           string      `json:"title"`
	Description     string      `json:"description"`
	OwnerID         uuid.UUID   `json:"owner_id"`
	CollaboratorIDs []uuid.UUID `json:"collaborator_ids"`
	Priority        string      `json:"priority"`
	PlannedStartAt  *time.Time  `json:"planned_start_at"`
	PlannedEndAt    *time.Time  `json:"planned_end_at"`
}
type CreateInput struct {
	TaskFields
	ParentID *uuid.UUID `json:"parent_id"`
	Weight   int        `json:"weight"`
}
type EditInput struct {
	TaskFields
	ExpectedVersion int `json:"expected_version"`
}
type Task struct {
	TaskFields
	ID                         uuid.UUID  `json:"id"`
	TeamID                     uuid.UUID  `json:"team_id"`
	ProjectID                  uuid.UUID  `json:"project_id"`
	ParentID                   *uuid.UUID `json:"parent_id"`
	Depth                      int        `json:"depth"`
	Status                     string     `json:"status"`
	ActualProgress             int        `json:"actual_progress"`
	EffectiveProgressBPS       int        `json:"effective_progress_bps"`
	Weight                     int        `json:"weight"`
	IsLeaf                     bool       `json:"is_leaf"`
	Blocked                    bool       `json:"blocked"`
	CanExecute                 bool       `json:"can_execute"`
	CanCancel                  bool       `json:"can_cancel"`
	CurrentSubmissionVersionID *uuid.UUID `json:"current_submission_version_id"`
	AcceptedReviewID           *uuid.UUID `json:"accepted_review_id"`
	CanSubmit                  bool       `json:"can_submit"`
	CanReview                  bool       `json:"can_review"`
	acceptance                 Acceptance
	submissionAuthor           *uuid.UUID
	CancellationReason         string    `json:"cancellation_reason"`
	Version                    int       `json:"version"`
	CreatedBy                  uuid.UUID `json:"created_by"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}
type Dependency struct {
	ID            uuid.UUID  `json:"id"`
	TeamID        uuid.UUID  `json:"team_id"`
	ProjectID     uuid.UUID  `json:"project_id"`
	TaskID        uuid.UUID  `json:"task_id"`
	PredecessorID uuid.UUID  `json:"predecessor_id"`
	CreatedBy     uuid.UUID  `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	IgnoredBy     *uuid.UUID `json:"ignored_by"`
	IgnoredAt     *time.Time `json:"ignored_at"`
	IgnoreReason  string     `json:"ignore_reason"`
}
type Progress struct {
	ProgressBPS int    `json:"progress_bps"`
	WeightedSum string `json:"weighted_sum"`
	TotalWeight string `json:"total_weight"`
	LeafCount   int    `json:"leaf_count"`
}
type Tree struct {
	Tasks        []Task       `json:"tasks"`
	Dependencies []Dependency `json:"dependencies"`
	Progress     Progress     `json:"progress"`
}
type History struct {
	ID               int64     `json:"id"`
	TaskID           uuid.UUID `json:"task_id"`
	ActorID          uuid.UUID `json:"actor_id"`
	EventType        string    `json:"event_type"`
	OldStatus        string    `json:"old_status"`
	NewStatus        string    `json:"new_status"`
	OldProgress      int       `json:"old_progress"`
	NewProgress      int       `json:"new_progress"`
	OldWeight        int       `json:"old_weight"`
	NewWeight        int       `json:"new_weight"`
	Reason           string    `json:"reason"`
	BeforeProjectBPS int       `json:"before_project_bps"`
	AfterProjectBPS  int       `json:"after_project_bps"`
	RequestID        string    `json:"request_id"`
	CreatedAt        time.Time `json:"created_at"`
}
type Snapshot struct {
	ID        int64     `json:"id"`
	TaskID    uuid.UUID `json:"task_id"`
	ActorID   uuid.UUID `json:"actor_id"`
	EventType string    `json:"event_type"`
	Progress  Progress  `json:"progress"`
	RequestID string    `json:"request_id"`
	CreatedAt time.Time `json:"created_at"`
}

// EffectiveBPS deliberately cannot treat a completed string as acceptance.
// Only EffectiveWithEvidence may elevate a completed task to 100%.
func EffectiveBPS(status string, actual int) (int, error) {
	if actual < 0 || actual > 99 {
		return 0, fmt.Errorf("actual progress out of range")
	}
	switch status {
	case Pending, Rejected, Cancelled, Completed:
		return 0, nil
	case InProgress:
		if actual > 79 {
			actual = 79
		}
		return actual * 100, nil
	case AwaitingReview:
		return 8000, nil
	default:
		return 0, fmt.Errorf("unknown task status")
	}
}

type WeightedLeaf struct {
	Weight         int
	Status         string
	ActualProgress int
}

// Aggregate is pure, deterministic and exact. WeightedSum is weight*bps.
func Aggregate(leaves []WeightedLeaf) (Progress, error) {
	sum, total := new(big.Int), new(big.Int)
	count := 0
	for _, l := range leaves {
		if l.Weight < 1 || l.Weight > 1000000 {
			return Progress{}, fmt.Errorf("invalid weight")
		}
		effective, err := EffectiveBPS(l.Status, l.ActualProgress)
		if err != nil {
			return Progress{}, err
		}
		if l.Status == Cancelled {
			continue
		}
		count++
		w := big.NewInt(int64(l.Weight))
		total.Add(total, w)
		sum.Add(sum, new(big.Int).Mul(w, big.NewInt(int64(effective))))
	}
	p := Progress{WeightedSum: sum.String(), TotalWeight: total.String(), LeafCount: count}
	if total.Sign() > 0 {
		p.ProgressBPS = int(new(big.Int).Quo(sum, total).Int64())
	}
	return p, nil
}

// Acceptance is read from the scoped database evidence join, never a status flag.
type Acceptance struct {
	CurrentVersionID  uuid.UUID
	ReviewedVersionID uuid.UUID
	ReviewID          uuid.UUID
	Decision          string
}

func (a Acceptance) valid() bool {
	return a.CurrentVersionID != uuid.Nil && a.CurrentVersionID == a.ReviewedVersionID && a.ReviewID != uuid.Nil && a.Decision == "approve"
}
func EffectiveWithEvidence(status string, actual int, proof Acceptance) (int, error) {
	base, e := EffectiveBPS(status, actual)
	if e == nil && status == Completed && proof.valid() {
		return 10000, nil
	}
	return base, e
}

type EvidenceLeaf struct {
	WeightedLeaf
	Acceptance Acceptance
}

func AggregateEvidence(leaves []EvidenceLeaf) (Progress, error) {
	sum, total := new(big.Int), new(big.Int)
	count := 0
	for _, l := range leaves {
		if l.Weight < 1 || l.Weight > 1000000 {
			return Progress{}, fmt.Errorf("invalid weight")
		}
		effective, e := EffectiveWithEvidence(l.Status, l.ActualProgress, l.Acceptance)
		if e != nil {
			return Progress{}, e
		}
		if l.Status == Cancelled {
			continue
		}
		count++
		w := big.NewInt(int64(l.Weight))
		total.Add(total, w)
		sum.Add(sum, new(big.Int).Mul(w, big.NewInt(int64(effective))))
	}
	p := Progress{WeightedSum: sum.String(), TotalWeight: total.String(), LeafCount: count}
	if total.Sign() > 0 {
		p.ProgressBPS = int(new(big.Int).Quo(sum, total).Int64())
	}
	return p, nil
}

type graph struct {
	nonLeaves map[uuid.UUID]bool
	tasks     map[uuid.UUID]*Task
	order     []uuid.UUID
	deps      []Dependency
	executed  map[uuid.UUID]bool
}

func (g *graph) leaf(id uuid.UUID) bool {
	if g.nonLeaves == nil {
		g.nonLeaves = map[uuid.UUID]bool{}
		for _, t := range g.tasks {
			if t.ParentID != nil {
				g.nonLeaves[*t.ParentID] = true
			}
		}
	}
	return !g.nonLeaves[id]
}

func (g *graph) descendant(id, ancestor uuid.UUID) bool {
	for n := 0; n < 4; n++ {
		t := g.tasks[id]
		if t == nil || t.ParentID == nil {
			return false
		}
		if *t.ParentID == ancestor {
			return true
		}
		id = *t.ParentID
	}
	return false
}
func (g *graph) progress(root *uuid.UUID) (Progress, error) {
	ls := []EvidenceLeaf{}
	for _, t := range g.tasks {
		if !g.leaf(t.ID) {
			continue
		}
		if root != nil && t.ID != *root && !g.descendant(t.ID, *root) {
			continue
		}
		ls = append(ls, EvidenceLeaf{WeightedLeaf{t.Weight, t.Status, t.ActualProgress}, t.acceptance})
	}
	return AggregateEvidence(ls)
}
func (g *graph) blocked(id uuid.UUID) bool {
	for _, d := range g.deps {
		if d.TaskID == id && d.IgnoredAt == nil && (g.tasks[d.PredecessorID] == nil || g.tasks[d.PredecessorID].Status != Completed || !g.tasks[d.PredecessorID].acceptance.valid()) {
			return true
		}
	}
	return false
}
func (g *graph) hasDependency(id uuid.UUID) bool {
	for _, d := range g.deps {
		if d.TaskID == id || d.PredecessorID == id {
			return true
		}
	}
	return false
}
func (g *graph) canBecomeParent(id uuid.UUID) bool {
	t := g.tasks[id]
	return t != nil && t.Status == Pending && t.ActualProgress == 0 && !g.executed[id] && !g.hasDependency(id)
}
func (g *graph) cycle(task, predecessor uuid.UUID) bool {
	seen := map[uuid.UUID]bool{}
	var visit func(uuid.UUID) bool
	visit = func(id uuid.UUID) bool {
		if id == task {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		for _, d := range g.deps {
			if d.TaskID == id && visit(d.PredecessorID) {
				return true
			}
		}
		return false
	}
	return visit(predecessor)
}
func transitionAllowed(from, to string) bool {
	return ((from == Pending || from == Rejected) && to == InProgress) || ((from == Pending || from == InProgress || from == Rejected) && to == Cancelled)
}
