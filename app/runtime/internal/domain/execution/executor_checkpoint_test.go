package execution

import (
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func checkpointSelection(t *testing.T, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New: %v", err)
	}
	return selection
}

func TestExecutorCheckpointValidatesOnlyApplicationEnvelope(t *testing.T) {
	valid := ExecutorCheckpoint{
		RootProcessID: "root",
		Payload:       []byte(`{"executorOwned":"opaque"}`),
		BuildID:       "sha256:build",
		Scope: TurnScope{
			SessionID:   "session-1",
			Cwd:         "/workspace/project",
			Isolated:    true,
			GoalLeaseID: "lease-1",
		},
		ModelSelection: checkpointSelection(t, "anthropic", "claude"),
		Limits: RunLimits{
			MaxTotalTokens: 4_096,
			MaxBudgetUSD:   1.5,
			MaxSteps:       8,
		},
		Usage: accounting.Snapshot{},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	for name, mutate := range map[string]func(*ExecutorCheckpoint){
		"empty root":          func(checkpoint *ExecutorCheckpoint) { checkpoint.RootProcessID = "" },
		"unstable root":       func(checkpoint *ExecutorCheckpoint) { checkpoint.RootProcessID = " root" },
		"empty payload":       func(checkpoint *ExecutorCheckpoint) { checkpoint.Payload = nil },
		"empty build":         func(checkpoint *ExecutorCheckpoint) { checkpoint.BuildID = "" },
		"unstable session":    func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.SessionID = " session-1" },
		"unstable cwd":        func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.Cwd = "/workspace/project " },
		"unstable goal lease": func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.GoalLeaseID = " lease-1" },
		"negative tokens":     func(checkpoint *ExecutorCheckpoint) { checkpoint.Limits.MaxTotalTokens = -1 },
		"negative cost":       func(checkpoint *ExecutorCheckpoint) { checkpoint.Limits.MaxBudgetUSD = -1 },
		"non-finite cost":     func(checkpoint *ExecutorCheckpoint) { checkpoint.Limits.MaxBudgetUSD = math.Inf(1) },
		"negative steps":      func(checkpoint *ExecutorCheckpoint) { checkpoint.Limits.MaxSteps = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			checkpoint := valid.Clone()
			mutate(&checkpoint)
			if err := checkpoint.Validate(); !errors.Is(err, ErrInvalidExecutorCheckpoint) {
				t.Fatalf("Validate error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}

func TestExecutorCheckpointCloneOwnsMutableData(t *testing.T) {
	original := ExecutorCheckpoint{
		RootProcessID: "root",
		Payload:       []byte("payload"),
		BuildID:       "build",
		Usage:         accounting.Snapshot{Models: []accounting.ModelUsage{{Model: "model"}}},
	}
	clone := original.Clone()
	clone.Payload[0] = 'P'
	clone.Usage.Models[0].Model = "changed"
	if string(original.Payload) != "payload" || original.Usage.Models[0].Model != "model" {
		t.Fatalf("Clone shares mutable storage with original: %+v", original)
	}
}

func TestExecutorCheckpointValidatesCrossAggregateOwnership(t *testing.T) {
	checkpoint := ExecutorCheckpoint{
		RootProcessID: "process-root",
		Payload:       []byte("opaque"),
		BuildID:       "build",
		Scope: TurnScope{
			SessionID: "session-1",
			Cwd:       "/workspace/project",
		},
		ModelSelection: checkpointSelection(t, "anthropic", "claude"),
	}
	expected := ExecutorCheckpointExpectation{
		RootProcessID:  "process-root",
		SessionID:      "session-1",
		Cwd:            "/workspace/project",
		ModelSelection: checkpointSelection(t, "anthropic", "claude"),
	}
	if err := checkpoint.ValidateFor(expected); err != nil {
		t.Fatalf("ValidateFor: %v", err)
	}

	for name, mutate := range map[string]func(*ExecutorCheckpointExpectation){
		"root":       func(value *ExecutorCheckpointExpectation) { value.RootProcessID = "other-root" },
		"session":    func(value *ExecutorCheckpointExpectation) { value.SessionID = "other-session" },
		"cwd":        func(value *ExecutorCheckpointExpectation) { value.Cwd = "/other/workspace" },
		"isolation":  func(value *ExecutorCheckpointExpectation) { value.Isolated = true },
		"goal lease": func(value *ExecutorCheckpointExpectation) { value.GoalLeaseID = "other-lease" },
		"provider": func(value *ExecutorCheckpointExpectation) {
			value.ModelSelection = checkpointSelection(t, "openai", "claude")
		},
		"model": func(value *ExecutorCheckpointExpectation) {
			value.ModelSelection = checkpointSelection(t, "anthropic", "claude-sonnet")
		},
		"limits": func(value *ExecutorCheckpointExpectation) {
			value.Limits.MaxTotalTokens++
		},
	} {
		t.Run(name, func(t *testing.T) {
			mismatch := expected
			mutate(&mismatch)
			if err := checkpoint.ValidateFor(mismatch); !errors.Is(err, ErrInvalidExecutorCheckpoint) {
				t.Fatalf("ValidateFor error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}
