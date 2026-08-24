package agentmemory

import (
	"fmt"
	"slices"
	"strings"
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
	plan, err := Fold(existing, []string{"- a", "- b", "- c", "- d", "- d", "  "})
	if err != nil {
		t.Fatal(err)
	}
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
	plan, err := Fold(existing, []string{"- e"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(plan.PruneIDs, []string{"ia"}) {
		t.Fatalf("PruneIDs = %v, want [ia]", plan.PruneIDs)
	}
	if !slices.Equal(plan.InsertContents, []string{"- e"}) {
		t.Fatalf("InsertContents = %v, want [- e]", plan.InsertContents)
	}
}

func TestFoldEmpty(t *testing.T) {
	if plan, err := Fold(nil, nil); err != nil || len(plan.InsertContents) != 0 || len(plan.PruneIDs) != 0 {
		t.Fatalf("empty fold = %+v", plan)
	}
}

func TestFoldRejectsUnboundedOrInvalidCurationOutput(t *testing.T) {
	contents := make([]string, MaxCurationProposals+1)
	for index := range contents {
		contents[index] = fmt.Sprintf("fact %d", index)
	}
	if _, err := Fold(nil, contents); err == nil {
		t.Fatal("oversized curation result was accepted")
	}
	if _, err := Fold(nil, []string{strings.Repeat("界", MaxContentCharacters+1)}); err == nil {
		t.Fatal("invalid curation content was accepted")
	}
}
