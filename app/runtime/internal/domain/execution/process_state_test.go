package execution

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
)

func TestProcessTreeStateValidateEnvelopeTopology(t *testing.T) {
	startedAt := time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC)
	valid := ProcessTreeState{
		RootID: "root",
		Processes: []ProcessState{
			{ID: "root", StartedAt: startedAt, Payload: []byte("opaque-root")},
			{ID: "child", ParentID: "root", StartedAt: startedAt, Payload: []byte("opaque-child")},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	for name, mutate := range map[string]func(*ProcessTreeState){
		"missing root": func(tree *ProcessTreeState) { tree.RootID = "missing" },
		"empty payload": func(tree *ProcessTreeState) {
			tree.Processes[1].Payload = nil
		},
		"external parent": func(tree *ProcessTreeState) {
			tree.Processes[1].ParentID = "missing"
		},
		"duplicate process": func(tree *ProcessTreeState) {
			tree.Processes = append(tree.Processes, tree.Processes[1])
		},
	} {
		t.Run(name, func(t *testing.T) {
			tree := valid
			tree.Processes = append([]ProcessState(nil), valid.Processes...)
			mutate(&tree)
			if err := tree.Validate(); !errors.Is(err, ErrInvalidProcessTreeState) {
				t.Fatalf("Validate error = %v, want ErrInvalidProcessTreeState", err)
			}
		})
	}
}

func TestProcessCheckpointValidateOwnsHostContinuationMetadata(t *testing.T) {
	valid := ProcessCheckpoint{
		BuildID: "sha256:build",
		Scope: TurnScope{
			SessionID:   "session-1",
			Cwd:         "/workspace/project",
			Isolated:    true,
			GoalLeaseID: "lease-1",
		},
		Provider: "anthropic",
		Budget: accounting.Budget{
			MaxTokens:  4_096,
			MaxCostUSD: 1.5,
			MaxSteps:   8,
		},
		Usage: accounting.Snapshot{},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	for name, mutate := range map[string]func(*ProcessCheckpoint){
		"empty build":         func(checkpoint *ProcessCheckpoint) { checkpoint.BuildID = "" },
		"unstable session":    func(checkpoint *ProcessCheckpoint) { checkpoint.Scope.SessionID = " session-1" },
		"unstable cwd":        func(checkpoint *ProcessCheckpoint) { checkpoint.Scope.Cwd = "/workspace/project " },
		"unstable goal lease": func(checkpoint *ProcessCheckpoint) { checkpoint.Scope.GoalLeaseID = " lease-1" },
		"unstable provider":   func(checkpoint *ProcessCheckpoint) { checkpoint.Provider = "anthropic " },
		"negative tokens":     func(checkpoint *ProcessCheckpoint) { checkpoint.Budget.MaxTokens = -1 },
		"negative cost":       func(checkpoint *ProcessCheckpoint) { checkpoint.Budget.MaxCostUSD = -1 },
		"non-finite cost":     func(checkpoint *ProcessCheckpoint) { checkpoint.Budget.MaxCostUSD = math.Inf(1) },
		"negative steps":      func(checkpoint *ProcessCheckpoint) { checkpoint.Budget.MaxSteps = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			checkpoint := valid
			mutate(&checkpoint)
			if err := checkpoint.Validate(); err == nil {
				t.Fatal("Validate accepted invalid checkpoint")
			}
		})
	}
}
