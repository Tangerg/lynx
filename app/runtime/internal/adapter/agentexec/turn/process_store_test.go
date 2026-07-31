package turn_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

type failNthSaveProcessStore struct {
	agentexec.ProcessStore
	failAt int32
	saves  atomic.Int32
	err    error
}

func (s *failNthSaveProcessStore) SaveTree(
	ctx context.Context,
	tree execution.ProcessTreeState,
	checkpoint execution.ProcessCheckpoint,
) error {
	if s.saves.Add(1) == s.failAt {
		return s.err
	}
	return s.ProcessStore.SaveTree(ctx, tree, checkpoint)
}

type memoryProcessStore struct {
	mu          sync.Mutex
	snapshots   map[string]execution.ProcessState
	checkpoints map[string]execution.ProcessCheckpoint
}

func newMemoryProcessStore() *memoryProcessStore {
	return &memoryProcessStore{
		snapshots:   make(map[string]execution.ProcessState),
		checkpoints: make(map[string]execution.ProcessCheckpoint),
	}
}

func (s *memoryProcessStore) SaveTree(_ context.Context, tree execution.ProcessTreeState, checkpoint execution.ProcessCheckpoint) error {
	if err := tree.Validate(); err != nil {
		return err
	}
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	prepared := make(map[string]execution.ProcessState, len(tree.Processes))
	for _, process := range tree.Processes {
		process.Payload = append([]byte(nil), process.Payload...)
		prepared[process.ID] = process
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleteTree(tree.RootID); err != nil {
		return err
	}
	for id, process := range prepared {
		s.snapshots[id] = process
	}
	s.checkpoints[tree.RootID] = checkpoint
	return nil
}

func (s *memoryProcessStore) LoadTree(_ context.Context, rootID string) (execution.ProcessTreeState, execution.ProcessCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.snapshots[rootID]; !ok {
		return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("%w: %s", execution.ErrProcessStateNotFound, rootID)
	}
	children := make(map[string][]string)
	for id, process := range s.snapshots {
		children[process.ParentID] = append(children[process.ParentID], id)
	}
	var processes []execution.ProcessState
	var collect func(string)
	collect = func(id string) {
		process := s.snapshots[id]
		process.Payload = append([]byte(nil), process.Payload...)
		processes = append(processes, process)
		for _, childID := range children[id] {
			collect(childID)
		}
	}
	collect(rootID)
	checkpoint := s.checkpoints[rootID]
	return execution.ProcessTreeState{RootID: rootID, Processes: processes}, checkpoint, nil
}

func (s *memoryProcessStore) DeleteTrees(_ context.Context, rootIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rootID := range rootIDs {
		if err := s.deleteTree(rootID); err != nil {
			return err
		}
	}
	return nil
}

func (s *memoryProcessStore) List(_ context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.snapshots))
	for id := range s.snapshots {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

func (s *memoryProcessStore) deleteTree(rootID string) error {
	children := make(map[string][]string)
	for id, process := range s.snapshots {
		children[process.ParentID] = append(children[process.ParentID], id)
	}
	var remove func(string)
	remove = func(id string) {
		delete(s.snapshots, id)
		delete(s.checkpoints, id)
		for _, childID := range children[id] {
			remove(childID)
		}
	}
	remove(rootID)
	return nil
}
