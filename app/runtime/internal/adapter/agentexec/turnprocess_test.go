package agentexec

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
)

func TestDiscardProcessTreeUsesStableDescendantFirstOrder(t *testing.T) {
	store := &discardProcessStore{
		list: []string{"outside", "child_b", "root", "grandchild", "child_a"},
		snapshots: map[string]core.ProcessSnapshot{
			"root":       {ID: "root"},
			"child_a":    {ID: "child_a", ParentID: "root"},
			"child_b":    {ID: "child_b", ParentID: "root"},
			"grandchild": {ID: "grandchild", ParentID: "child_a"},
			"outside":    {ID: "outside"},
		},
	}
	engine := &discardProcessEngine{store: store}

	if err := discardProcessTree(t.Context(), "root", engine); err != nil {
		t.Fatalf("discardProcessTree: %v", err)
	}
	if want := []string{"grandchild", "child_a", "child_b", "root"}; !slices.Equal(store.deleted, want) {
		t.Fatalf("deleted = %v, want %v", store.deleted, want)
	}
	if len(engine.removed) != 0 {
		t.Fatalf("removed durable-only processes = %v, want none", engine.removed)
	}
}

func TestDiscardProcessTreePreservesSnapshotsWhenDiscoveryFails(t *testing.T) {
	listErr := errors.New("snapshot listing failed")
	store := &discardProcessStore{
		listErr: listErr,
	}

	err := discardProcessTree(t.Context(), "root", &discardProcessEngine{store: store})
	if !errors.Is(err, listErr) {
		t.Fatalf("discardProcessTree error = %v, want listing failure", err)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want no partial snapshot cleanup", store.deleted)
	}
}

func TestDiscardProcessTreeReportsDeletionFailureAndPreservesAncestors(t *testing.T) {
	deleteErr := errors.New("snapshot deletion failed")
	store := &discardProcessStore{
		list: []string{"root", "child"},
		snapshots: map[string]core.ProcessSnapshot{
			"root":  {ID: "root"},
			"child": {ID: "child", ParentID: "root"},
		},
		deleteErr: map[string]error{"child": deleteErr},
	}

	err := discardProcessTree(t.Context(), "root", &discardProcessEngine{store: store})
	if !errors.Is(err, deleteErr) {
		t.Fatalf("discardProcessTree error = %v, want deletion failure", err)
	}
	if want := []string{"child"}; !slices.Equal(store.deleted, want) {
		t.Fatalf("deleted = %v, want %v (root must retain the failed child link)", store.deleted, want)
	}
}

type discardProcessEngine struct {
	store   core.ProcessStore
	removed []string
}

func (*discardProcessEngine) Processes() []*agentruntime.Process { return nil }
func (e *discardProcessEngine) ProcessStore() core.ProcessStore  { return e.store }
func (*discardProcessEngine) Kill(context.Context, string) error {
	panic("unexpected Kill")
}
func (e *discardProcessEngine) Remove(id string) error {
	e.removed = append(e.removed, id)
	return nil
}

type discardProcessStore struct {
	list      []string
	listErr   error
	snapshots map[string]core.ProcessSnapshot
	loadErr   map[string]error
	deleteErr map[string]error
	deleted   []string
}

func (*discardProcessStore) Save(context.Context, core.ProcessSnapshot) error {
	panic("unused")
}

func (s *discardProcessStore) Load(_ context.Context, id string) (core.ProcessSnapshot, error) {
	if err := s.loadErr[id]; err != nil {
		return core.ProcessSnapshot{}, err
	}
	snapshot, ok := s.snapshots[id]
	if !ok {
		return core.ProcessSnapshot{}, core.ErrSnapshotNotFound
	}
	return snapshot, nil
}

func (s *discardProcessStore) List(context.Context) ([]string, error) {
	return slices.Clone(s.list), s.listErr
}

func (s *discardProcessStore) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return s.deleteErr[id]
}
