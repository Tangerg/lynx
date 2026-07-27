package execution

import (
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
