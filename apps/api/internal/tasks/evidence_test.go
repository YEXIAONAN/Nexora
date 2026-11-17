package tasks

import (
	"github.com/google/uuid"
	"testing"
)

func TestCompletionRequiresCurrentVersionApproval(t *testing.T) {
	v, old, r := uuid.New(), uuid.New(), uuid.New()
	for _, proof := range []Acceptance{{}, {v, old, r, "approve"}, {v, v, r, "reject"}, {v, v, uuid.Nil, "approve"}} {
		if got, e := EffectiveWithEvidence(Completed, 99, proof); e != nil || got != 0 {
			t.Fatal(got, e)
		}
	}
	proof := Acceptance{v, v, r, "approve"}
	if got, e := EffectiveWithEvidence(Completed, 99, proof); e != nil || got != 10000 {
		t.Fatal(got, e)
	}
	p, e := AggregateEvidence([]EvidenceLeaf{{WeightedLeaf{3, Completed, 99}, proof}, {WeightedLeaf{1, Rejected, 99}, proof}, {WeightedLeaf{200, Cancelled, 99}, proof}})
	if e != nil || p.ProgressBPS != 7500 || p.TotalWeight != "4" {
		t.Fatal(p, e)
	}
}
