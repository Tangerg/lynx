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
			config: Config{ChatClient: client, ProcessStore: newJSONProcessStore()},
			want:   "BuildID",
		},
		{
			name:   "development fallback",
			config: Config{ChatClient: client, ProcessStore: newJSONProcessStore(), BuildID: "dev"},
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
		ChatClient:   client,
		ProcessStore: newJSONProcessStore(),
		BuildID:      testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.RestoreTurn(t.Context(), "missing", RestoreTurnRequest{})
	if process != nil || !errors.Is(err, ErrProcessSnapshotLost) || !errors.Is(err, execution.ErrProcessSnapshotNotFound) {
		t.Fatalf("RestoreTurn = (%T, %v), want snapshot loss wrapping not found", process, err)
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
	store := newJSONProcessStore()
	engine, err := New(t.Context(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		ProcessStore: store,
		BuildID:      testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.StartTurn(t.Context(), TurnRequest{
		Message:  "pause for approval",
		Observer: &hitlApprovalObserver{},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if completion := process.Await(); completion.Error() != nil {
		t.Fatalf("initial turn: %v", completion.Error())
	}

	store.mu.Lock()
	corrupt := store.usages[process.ID()]
	corrupt.Models = append([]accounting.ModelUsage(nil), corrupt.Models...)
	corrupt.Models[0].Calls++
	store.usages[process.ID()] = corrupt
	store.mu.Unlock()

	if resumable, err := engine.ResumableProcess(t.Context(), process.ID()); err != nil || resumable {
		t.Fatalf("ResumableProcess = %v, %v; want false, nil", resumable, err)
	}
	restored, err := engine.RestoreTurn(t.Context(), process.ID(), RestoreTurnRequest{
		Observer: &hitlApprovalObserver{},
	})
	if restored != nil || !errors.Is(err, ErrProcessSnapshotLost) || !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("RestoreTurn = (%T, %v), want snapshot loss wrapping ErrInvalidSnapshot", restored, err)
	}
}

func TestWaitingSnapshotCommitFailureDoesNotRewriteAgentState(t *testing.T) {
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
	want := errors.New("snapshot unavailable")
	store := &failingProcessStore{err: want}
	engine, err := New(t.Context(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		ProcessStore: store,
		BuildID:      testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.StartTurn(t.Context(), TurnRequest{
		Message:  "pause for approval",
		Observer: &hitlApprovalObserver{},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	completion := process.Await()
	if !errors.Is(completion.Error(), want) {
		t.Fatalf("completion error = %v, want snapshot failure", completion.Error())
	}
	if completion.Status != core.StatusWaiting {
		t.Fatalf("process status = %s, want waiting", completion.Status)
	}
	if store.saves.Load() == 0 {
		t.Fatal("process never attempted a segment-boundary snapshot")
	}
	if errors.Is(completion.Error(), ErrProcessSnapshotLost) {
		t.Fatalf("active snapshot write failure was misclassified as restore loss: %v", completion.Error())
	}
}

func TestTerminalSegmentDoesNotPersistUnresumableSnapshot(t *testing.T) {
	client, err := chatclient.New(newStreamingStubModel("done"))
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	store := &failingProcessStore{err: errors.New("must not be called")}
	engine, err := New(t.Context(), Config{
		ChatClient:   client,
		ProcessStore: store,
		BuildID:      testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.StartTurn(t.Context(), TurnRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	completion := process.Await()
	if err := completion.Error(); err != nil {
		t.Fatalf("terminal completion: %v", err)
	}
	if completion.Status != core.StatusCompleted {
		t.Fatalf("process status = %s, want completed", completion.Status)
	}
	if got := store.saves.Load(); got != 0 {
		t.Fatalf("terminal snapshot saves = %d, want 0", got)
	}
}

type failingProcessStore struct {
	saves atomic.Int32
	err   error
}

func (s *failingProcessStore) SaveTree(context.Context, core.ProcessSnapshotTree, execution.ProcessCheckpoint) error {
	s.saves.Add(1)
	return s.err
}

func (*failingProcessStore) LoadTree(context.Context, string) (core.ProcessSnapshotTree, execution.ProcessCheckpoint, error) {
	return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, execution.ErrProcessSnapshotNotFound
}

func (*failingProcessStore) DeleteTrees(context.Context, []string) error { return nil }
