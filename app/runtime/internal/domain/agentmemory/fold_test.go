package agentmemory

import (
	"slices"
	"testing"
)

func TestFoldProposesNewAndRespectsStatuses(t *testing.T) {
	existing := []Item{
		{ID: "ia", Content: "- a", Status: StatusPending},
		{ID: "ib", Content: "- b", Status: StatusActive},
		{ID: "ic", Content: "- c", Status: StatusRejected},
	}
	// The curator re-emits a/b/c and adds d (twice, plus a blank). Only the
	// genuinely new fact d becomes a proposal: a/b/c are already present in some
	// status, and a rejected tombstone (c) blocks re-proposal.
	plan := Fold(existing, []string{"- a", "- b", "- c", "- d", "- d", "  "})
	if !slices.Equal(plan.InsertContents, []string{"- d"}) {
		t.Fatalf("InsertContents = %v, want [- d]", plan.InsertContents)
	}
	if len(plan.PruneIDs) != 0 {
		t.Fatalf("PruneIDs = %v, want none (nothing dropped)", plan.PruneIDs)
	}
}

func TestFoldPrunesStalePendingButKeepsActiveAndRejected(t *testing.T) {
	existing := []Item{
		{ID: "ia", Content: "- a", Status: StatusPending},
		{ID: "ib", Content: "- b", Status: StatusActive},
		{ID: "ic", Content: "- c", Status: StatusRejected},
	}
	// The curator drops a, b, and c. Only the pending proposal a is pruned:
	// active b is sticky (the user accepted it), rejected c stays a tombstone.
	plan := Fold(existing, []string{"- e"})
	if !slices.Equal(plan.PruneIDs, []string{"ia"}) {
		t.Fatalf("PruneIDs = %v, want [ia]", plan.PruneIDs)
	}
	if !slices.Equal(plan.InsertContents, []string{"- e"}) {
		t.Fatalf("InsertContents = %v, want [- e]", plan.InsertContents)
	}
}

func TestFoldEmpty(t *testing.T) {
	if plan := Fold(nil, nil); len(plan.InsertContents) != 0 || len(plan.PruneIDs) != 0 {
		t.Fatalf("empty fold = %+v", plan)
	}
}
