package run_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
)

func TestRunTreeCanonicalPostorderAndSubtree(t *testing.T) {
	tree, err := run.NewTree("run_root", []run.TreeMember{
		{
			RunID: "run_b",
			Lineage: run.Lineage{
				SpawnedByItemID: "item_b",
				ParentRunID:     "run_root",
				RootRunID:       "run_root",
			},
		},
		{RunID: "run_root"},
		{
			RunID: "run_a1",
			Lineage: run.Lineage{
				SpawnedByItemID: "item_a1",
				ParentRunID:     "run_a",
				RootRunID:       "run_root",
			},
		},
		{
			RunID: "run_a",
			Lineage: run.Lineage{
				SpawnedByItemID: "item_a",
				ParentRunID:     "run_root",
				RootRunID:       "run_root",
			},
		},
		{
			RunID: "run_a0",
			Lineage: run.Lineage{
				SpawnedByItemID: "item_a0",
				ParentRunID:     "run_a",
				RootRunID:       "run_root",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewTree: %v", err)
	}
	wantTree := []string{"run_a0", "run_a1", "run_a", "run_b", "run_root"}
	if got := tree.Postorder(); !equalStrings(got, wantTree) {
		t.Fatalf("Postorder = %v, want %v", got, wantTree)
	}
	wantSubtree := []string{"run_a0", "run_a1", "run_a"}
	subtree, ok := tree.SubtreePostorder("run_a")
	if !ok || !equalStrings(subtree, wantSubtree) {
		t.Fatalf("SubtreePostorder(run_a) = %v, %t; want %v, true", subtree, ok, wantSubtree)
	}
	subtree[0] = "mutated"
	if got, _ := tree.SubtreePostorder("run_a"); !equalStrings(got, wantSubtree) {
		t.Fatalf("SubtreePostorder leaked mutable storage: %v", got)
	}
	complete := tree.Postorder()
	complete[0] = "mutated"
	if got := tree.Postorder(); !equalStrings(got, wantTree) {
		t.Fatalf("Postorder leaked mutable storage: %v", got)
	}
	if got, ok := tree.SubtreePostorder("run_missing"); ok || got != nil {
		t.Fatalf("SubtreePostorder(missing) = %v, %t; want nil, false", got, ok)
	}
}

func TestRunTreeRejectsInvalidTopology(t *testing.T) {
	child := func(runID, parentRunID, rootRunID string) run.TreeMember {
		return run.TreeMember{
			RunID: runID,
			Lineage: run.Lineage{
				SpawnedByItemID: "item_" + runID,
				ParentRunID:     parentRunID,
				RootRunID:       rootRunID,
			},
		}
	}
	tests := []struct {
		name    string
		root    string
		members []run.TreeMember
		want    string
	}{
		{name: "missing root id", members: []run.TreeMember{{RunID: "run_root"}}, want: "root run id is required"},
		{name: "no members", root: "run_root", want: "no members"},
		{
			name:    "duplicate run",
			root:    "run_root",
			members: []run.TreeMember{{RunID: "run_root"}, {RunID: "run_root"}},
			want:    "duplicate run",
		},
		{
			name:    "unexpected root lineage",
			root:    "run_root",
			members: []run.TreeMember{{RunID: "run_root"}, {RunID: "run_other"}},
			want:    "has root lineage",
		},
		{
			name:    "declared root carries child lineage",
			root:    "run_root",
			members: []run.TreeMember{child("run_root", "run_parent", "run_other")},
			want:    "root run \"run_root\" carries child lineage",
		},
		{
			name:    "wrong tree root",
			root:    "run_root",
			members: []run.TreeMember{{RunID: "run_root"}, child("run_child", "run_root", "run_other")},
			want:    "names root \"run_other\", want \"run_root\"",
		},
		{
			name:    "unknown parent",
			root:    "run_root",
			members: []run.TreeMember{{RunID: "run_root"}, child("run_child", "run_missing", "run_root")},
			want:    "names unknown parent",
		},
		{
			name: "cycle",
			root: "run_root",
			members: []run.TreeMember{
				{RunID: "run_root"},
				child("run_a", "run_b", "run_root"),
				child("run_b", "run_a", "run_root"),
			},
			want: "contains a cycle",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := run.NewTree(test.root, test.members)
			if !errors.Is(err, run.ErrInvalidTree) ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewTree error = %v, want ErrInvalidTree containing %q", err, test.want)
			}
		})
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
