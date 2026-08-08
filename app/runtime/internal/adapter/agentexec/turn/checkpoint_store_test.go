package turn_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

const testProcessBuildID = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func validWaitingCheckpoint() agentexec.WaitingCheckpoint {
	return agentexec.WaitingCheckpoint{Checkpoint: runs.ExecutorCheckpoint{
		RootProcessID: "process_root",
		Payload:       []byte(`{"root":"process_root"}`),
		BuildID:       testProcessBuildID,
	}}
}

type testCheckpointStore interface {
	LoadCheckpoint(context.Context, string) (runs.ExecutorCheckpoint, error)
	SaveCheckpoint(context.Context, runs.ExecutorCheckpoint) error
	DeleteCheckpoints(context.Context, string, []string) error
}

func persistTreeBarrier(t testing.TB, barrier runs.TreeInterrupted, stores ...testCheckpointStore) {
	t.Helper()
	if err := barrier.Checkpoint.Validate(); err != nil {
		t.Fatalf("tree barrier checkpoint: %v", err)
	}
	for _, store := range stores {
		if err := store.SaveCheckpoint(context.Background(), barrier.Checkpoint); err != nil {
			t.Fatalf("save tree barrier checkpoint: %v", err)
		}
	}
}

type failNthSaveCheckpointStore struct {
	testCheckpointStore
	failAt int32
	saves  atomic.Int32
	err    error
}

func (s *failNthSaveCheckpointStore) SaveCheckpoint(
	ctx context.Context,
	checkpoint runs.ExecutorCheckpoint,
) error {
	if s.saves.Add(1) == s.failAt {
		return s.err
	}
	return s.testCheckpointStore.SaveCheckpoint(ctx, checkpoint)
}

type memoryCheckpointStore struct {
	mu          sync.Mutex
	checkpoints map[string]runs.ExecutorCheckpoint
}

func newMemoryCheckpointStore() *memoryCheckpointStore {
	return &memoryCheckpointStore{checkpoints: make(map[string]runs.ExecutorCheckpoint)}
}

func (s *memoryCheckpointStore) SaveCheckpoint(_ context.Context, checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if stored, exists := s.checkpoints[checkpoint.RootProcessID]; exists &&
		(stored.Scope != checkpoint.Scope || stored.BuildID != checkpoint.BuildID ||
			stored.ModelSelection != checkpoint.ModelSelection || stored.Limits != checkpoint.Limits) {
		return runs.ErrInvalidExecutorCheckpoint
	}
	s.checkpoints[checkpoint.RootProcessID] = checkpoint.Clone()
	return nil
}

func (s *memoryCheckpointStore) LoadCheckpoint(_ context.Context, rootID string) (runs.ExecutorCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	checkpoint, ok := s.checkpoints[rootID]
	if !ok {
		return runs.ExecutorCheckpoint{}, fmt.Errorf("%w: %s", runs.ErrExecutorCheckpointNotFound, rootID)
	}
	return checkpoint.Clone(), nil
}

func (s *memoryCheckpointStore) DeleteCheckpoints(_ context.Context, sessionID string, rootIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rootID := range rootIDs {
		if checkpoint, exists := s.checkpoints[rootID]; exists && checkpoint.Scope.SessionID != sessionID {
			return runs.ErrInvalidExecutorCheckpoint
		}
	}
	for _, rootID := range rootIDs {
		delete(s.checkpoints, rootID)
	}
	return nil
}

func (s *memoryCheckpointStore) List(_ context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.checkpoints))
	for id := range s.checkpoints {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}
