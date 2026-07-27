package turn_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
)

type memoryProcessStore struct {
	mu        sync.Mutex
	snapshots map[string]json.RawMessage
	buildIDs  map[string]string
	scopes    map[string]execution.TurnScope
	usages    map[string]accounting.Snapshot
}

func newMemoryProcessStore() *memoryProcessStore {
	return &memoryProcessStore{
		snapshots: make(map[string]json.RawMessage),
		buildIDs:  make(map[string]string),
		scopes:    make(map[string]execution.TurnScope),
		usages:    make(map[string]accounting.Snapshot),
	}
}

func (s *memoryProcessStore) SaveTree(_ context.Context, tree core.ProcessSnapshotTree, checkpoint execution.ProcessCheckpoint) error {
	if err := tree.Validate(); err != nil {
		return err
	}
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	prepared := make(map[string]json.RawMessage, len(tree.Snapshots))
	for _, snapshot := range tree.Snapshots {
		data, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		prepared[snapshot.ID] = data
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleteTree(tree.RootID); err != nil {
		return err
	}
	for id, data := range prepared {
		s.snapshots[id] = data
		s.buildIDs[id] = checkpoint.BuildID
	}
	s.scopes[tree.RootID] = checkpoint.Scope
	s.usages[tree.RootID] = checkpoint.Usage
	return nil
}

func (s *memoryProcessStore) LoadTree(_ context.Context, rootID string) (core.ProcessSnapshotTree, execution.ProcessCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.snapshots[rootID]; !ok {
		return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("%w: %s", execution.ErrProcessSnapshotNotFound, rootID)
	}
	buildID := s.buildIDs[rootID]
	children := make(map[string][]string)
	decoded := make(map[string]core.ProcessSnapshot, len(s.snapshots))
	for id, data := range s.snapshots {
		var snapshot core.ProcessSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, err
		}
		decoded[id] = snapshot
		children[snapshot.ParentID] = append(children[snapshot.ParentID], id)
	}
	var snapshots []core.ProcessSnapshot
	var collect func(string)
	collect = func(id string) {
		snapshots = append(snapshots, decoded[id])
		for _, childID := range children[id] {
			collect(childID)
		}
	}
	collect(rootID)
	checkpoint := execution.ProcessCheckpoint{BuildID: buildID, Scope: s.scopes[rootID], Usage: s.usages[rootID]}
	return core.ProcessSnapshotTree{RootID: rootID, Snapshots: snapshots}, checkpoint, nil
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
	for id, data := range s.snapshots {
		var snapshot core.ProcessSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return err
		}
		children[snapshot.ParentID] = append(children[snapshot.ParentID], id)
	}
	var remove func(string)
	remove = func(id string) {
		delete(s.snapshots, id)
		delete(s.buildIDs, id)
		delete(s.scopes, id)
		delete(s.usages, id)
		for _, childID := range children[id] {
			remove(childID)
		}
	}
	remove(rootID)
	return nil
}
