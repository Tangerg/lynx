package run_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

func TestRunLineageAcceptsExactlyRootOrCompleteChild(t *testing.T) {
	tests := []struct {
		name    string
		lineage run.RunLineage
		wantErr string
	}{
		{name: "root"},
		{
			name: "child",
			lineage: run.RunLineage{
				SpawnedByItemID: "item_spawn",
				ParentRunID:     "run_parent",
				RootRunID:       "run_root",
			},
		},
		{
			name: "partial child",
			lineage: run.RunLineage{
				SpawnedByItemID: "item_spawn",
				ParentRunID:     "run_parent",
			},
			wantErr: "requires spawnedByItemId, parentRunId, and rootRunId together",
		},
		{
			name: "self parent",
			lineage: run.RunLineage{
				SpawnedByItemID: "item_spawn",
				ParentRunID:     "run_child",
				RootRunID:       "run_root",
			},
			wantErr: "is its own parent",
		},
		{
			name: "self root",
			lineage: run.RunLineage{
				SpawnedByItemID: "item_spawn",
				ParentRunID:     "run_parent",
				RootRunID:       "run_child",
			},
			wantErr: "is its own root",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.lineage.Validate("run_child")
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if !errors.Is(err, run.ErrInvalidRunLineage) ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate error = %v, want ErrInvalidRunLineage containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRunLineageResolvesTreeRootWithoutGuessingFromTheParent(t *testing.T) {
	root := run.RunLineage{}
	if got := root.TreeRootID("run_root"); got != "run_root" {
		t.Fatalf("root TreeRootID = %q, want run_root", got)
	}
	child := run.RunLineage{
		SpawnedByItemID: "item_spawn",
		ParentRunID:     "run_parent",
		RootRunID:       "run_root",
	}
	if got := child.TreeRootID("run_child"); got != "run_root" {
		t.Fatalf("child TreeRootID = %q, want run_root", got)
	}
}
