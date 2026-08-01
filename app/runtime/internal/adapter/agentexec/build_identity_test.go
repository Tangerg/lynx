package agentexec

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/chatclient"
)

const (
	testBuildID      = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	alternateBuildID = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
)

func TestNewRequiresContentBuildIdentityForDurableRuntime(t *testing.T) {
	client, err := chatclient.New(newStreamingStubModel("done"))
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}

	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{
			name:   "missing",
			config: Config{ChatClient: client, Checkpoints: newMemoryCheckpointStore()},
			want:   "BuildID",
		},
		{
			name:   "development fallback",
			config: Config{ChatClient: client, Checkpoints: newMemoryCheckpointStore(), BuildID: "dev"},
			want:   "BuildID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(context.Background(), test.config)
			if err == nil {
				t.Fatalf("New error = nil, want %s rejection", test.want)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want detail %q", err, test.want)
			}
		})
	}
}

func TestRestoreTurnMissingSnapshotIsStateLoss(t *testing.T) {
	client, err := chatclient.New(newStreamingStubModel("done"))
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	engine, err := New(t.Context(), Config{
		ChatClient:  client,
		Checkpoints: newMemoryCheckpointStore(),
		BuildID:     testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.RestoreTurn(t.Context(), "missing", RestoreTurnRequest{})
	if process != nil || !errors.Is(err, ErrExecutorCheckpointLost) || !errors.Is(err, execution.ErrExecutorCheckpointNotFound) {
		t.Fatalf("RestoreTurn = (%T, %v), want checkpoint loss wrapping not found", process, err)
	}
}

func TestRestoreRejectsUsageProjectionThatDriftedFromProcessTree(t *testing.T) {
	model := newOptionToolStub()
	client, err := chatclient.New(model, chatclient.WithDefaults(*model.defaults))
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupBuiltTools(t, built)
	store := newMemoryCheckpointStore()
	engine, err := New(t.Context(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		Checkpoints:  store,
		BuildID:      testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.StartTurn(t.Context(), TurnRequest{
		SessionID: "session-usage",
		Cwd:       "/workspace/usage",
		Message:   "pause for approval",
		Observer:  &hitlApprovalObserver{},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if completion := process.Await(); completion.Err != nil {
		t.Fatalf("initial turn: %v", completion.Err)
	}
	persistWaitingCheckpoint(t, store, process)

	store.mu.Lock()
	checkpoint := store.checkpoints[process.ID()]
	corrupt := checkpoint.Usage
	corrupt.Models = append([]accounting.ModelUsage(nil), corrupt.Models...)
	corrupt.Models[0].Calls++
	checkpoint.Usage = corrupt
	store.checkpoints[process.ID()] = checkpoint
	store.mu.Unlock()

	if resumable, err := engine.CanResumeCheckpoint(t.Context(), expectationForCheckpoint(checkpoint)); err != nil || resumable {
		t.Fatalf("CanResumeCheckpoint = %v, %v; want false, nil", resumable, err)
	}
	restored, err := engine.RestoreTurn(t.Context(), process.ID(), RestoreTurnRequest{
		SessionID:      checkpoint.Scope.SessionID,
		ModelSelection: checkpoint.ModelSelection,
		Cwd:            checkpoint.Scope.Cwd,
		Isolated:       checkpoint.Scope.Isolated,
		GoalLeaseID:    checkpoint.Scope.GoalLeaseID,
		Limits:         checkpoint.Limits,
		Observer:       &hitlApprovalObserver{},
	})
	if restored != nil || !errors.Is(err, ErrExecutorCheckpointLost) || !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("RestoreTurn = (%T, %v), want checkpoint loss wrapping ErrInvalidSnapshot", restored, err)
	}
}

func TestWaitingCheckpointCaptureDoesNotReadCheckpointStorage(t *testing.T) {
	model := newOptionToolStub()
	client, err := chatclient.New(model, chatclient.WithDefaults(*model.defaults))
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupBuiltTools(t, built)
	reader := &checkpointReaderProbe{}
	engine, err := New(t.Context(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		Checkpoints:  reader,
		BuildID:      testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.StartTurn(t.Context(), TurnRequest{
		SessionID: "session-capture",
		Message:   "pause for approval",
		Observer:  &hitlApprovalObserver{},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	completion := process.Await()
	if completion.Err != nil {
		t.Fatalf("waiting completion = %v, want checkpoint policy outside Await", completion.Err)
	}
	if completion.Status != core.StatusWaiting {
		t.Fatalf("process status = %s, want waiting", completion.Status)
	}
	checkpoint := captureWaitingCheckpoint(t, process)
	if got := reader.loads.Load(); got != 0 {
		t.Fatalf("checkpoint reads before capture = %d, want 0", got)
	}
	if err := checkpoint.Checkpoint.Validate(); err != nil {
		t.Fatalf("captured checkpoint: %v", err)
	}
	if got := reader.loads.Load(); got != 0 {
		t.Fatalf("checkpoint reads after capture = %d, want 0", got)
	}
}

func TestTerminalSegmentDoesNotReadCheckpointStorage(t *testing.T) {
	client, err := chatclient.New(newStreamingStubModel("done"))
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	reader := &checkpointReaderProbe{}
	engine, err := New(t.Context(), Config{
		ChatClient:  client,
		Checkpoints: reader,
		BuildID:     testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.StartTurn(t.Context(), TurnRequest{SessionID: "session-terminal", Message: "hello"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	completion := process.Await()
	if err := completion.Err; err != nil {
		t.Fatalf("terminal completion: %v", err)
	}
	if completion.Status != core.StatusCompleted {
		t.Fatalf("process status = %s, want completed", completion.Status)
	}
	if got := reader.loads.Load(); got != 0 {
		t.Fatalf("terminal checkpoint reads = %d, want 0", got)
	}
}

type checkpointReaderProbe struct {
	loads atomic.Int32
}

func (reader *checkpointReaderProbe) LoadCheckpoint(context.Context, string) (execution.ExecutorCheckpoint, error) {
	reader.loads.Add(1)
	return execution.ExecutorCheckpoint{}, execution.ErrExecutorCheckpointNotFound
}
