package execution

import (
	"math"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
)

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
