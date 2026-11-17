package tasks

import (
	"github.com/google/uuid"
	"testing"
)

func TestAggregateExactProgress(t *testing.T) {
	for _, tc := range []struct {
		name        string
		leaves      []WeightedLeaf
		bps         int
		sum, weight string
		count       int
	}{
		{"empty", nil, 0, "0", "0", 0},
		{"weighted", []WeightedLeaf{{1, InProgress, 60}, {3, InProgress, 20}}, 3000, "12000", "4", 2},
		{"cap", []WeightedLeaf{{1000000, InProgress, 99}}, 7900, "7900000000", "1000000", 1},
		{"cancelled excluded", []WeightedLeaf{{1, InProgress, 50}, {999, Cancelled, 99}}, 5000, "5000", "1", 1},
		{"pending zero", []WeightedLeaf{{2, Pending, 0}}, 0, "0", "2", 1},
		{"round down", []WeightedLeaf{{1, InProgress, 1}, {2, Pending, 0}}, 33, "100", "3", 2},
		{"future review", []WeightedLeaf{{1, AwaitingReview, 99}}, 8000, "8000", "1", 1},
		{"completion without evidence", []WeightedLeaf{{1, Completed, 99}, {1, Rejected, 99}}, 0, "0", "2", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, e := Aggregate(tc.leaves)
			if e != nil || p.ProgressBPS != tc.bps || p.WeightedSum != tc.sum || p.TotalWeight != tc.weight || p.LeafCount != tc.count {
				t.Fatalf("got %+v %v", p, e)
			}
		})
	}
	for _, l := range []WeightedLeaf{{0, Pending, 0}, {-1, Pending, 0}, {1000001, Pending, 0}, {1, "invalid", 0}} {
		if _, e := Aggregate([]WeightedLeaf{l}); e == nil {
			t.Fatal("invalid leaf accepted", l)
		}
	}
	ls := make([]WeightedLeaf, 10000)
	for i := range ls {
		ls[i] = WeightedLeaf{1000000, InProgress, 99}
	}
	p, e := Aggregate(ls)
	if e != nil || p.WeightedSum != "79000000000000" || p.ProgressBPS != 7900 {
		t.Fatal("large aggregate", p, e)
	}
}
func TestOnlyLeavesAndAllDescendantsCount(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	g := graph{tasks: map[uuid.UUID]*Task{a: {ID: a, Weight: 1000000, Status: Pending}, b: {ID: b, ParentID: &a, Weight: 999999, Status: Pending}, c: {ID: c, ParentID: &b, Weight: 3, Status: InProgress, ActualProgress: 60}}}
	p, e := g.progress(nil)
	if e != nil || p.TotalWeight != "3" || p.ProgressBPS != 6000 || p.LeafCount != 1 {
		t.Fatal(p, e)
	}
	p, e = g.progress(&a)
	if e != nil || p.ProgressBPS != 6000 {
		t.Fatal(p, e)
	}
}
func TestPhase03TransitionsOnly(t *testing.T) {
	for _, from := range []string{Pending, InProgress, AwaitingReview, Completed, Rejected, Cancelled} {
		for _, to := range []string{Pending, InProgress, AwaitingReview, Completed, Rejected, Cancelled} {
			want := (from == Pending && to == InProgress) || ((from == Pending || from == InProgress) && to == Cancelled)
			if transitionAllowed(from, to) != want {
				t.Fatalf("%s -> %s", from, to)
			}
		}
	}
}
