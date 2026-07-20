package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// recordingAgentMemory captures the management calls the agentMemory.* handlers
// make, so the delivery mapping (target resolution, decision → status, wire
// projection) can be asserted without a real store.
type recordingAgentMemory struct {
	listScope   agentmemory.Scope
	listProject string
	items       []agentmemory.Item

	statusID string
	status   agentmemory.Status
	pinnedID string
	pinned   bool
	editedID string
	editedTx string
	deleted  string

	addScope   agentmemory.Scope
	addProject string
	addContent string

	getItem agentmemory.Item
}

func (r *recordingAgentMemory) List(_ context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	r.listScope, r.listProject = scope, project
	return r.items, nil
}

func (r *recordingAgentMemory) Get(_ context.Context, _ string) (agentmemory.Item, bool, error) {
	return r.getItem, r.getItem.ID != "", nil
}

func (r *recordingAgentMemory) SetStatus(_ context.Context, id string, status agentmemory.Status, _ time.Time) error {
	r.statusID, r.status = id, status
	return nil
}

func (r *recordingAgentMemory) SetPinned(_ context.Context, id string, pinned bool, _ time.Time) error {
	r.pinnedID, r.pinned = id, pinned
	return nil
}

func (r *recordingAgentMemory) UpdateContent(_ context.Context, id, content string, _ time.Time) error {
	r.editedID, r.editedTx = id, content
	return nil
}

func (r *recordingAgentMemory) Delete(_ context.Context, id string) error {
	r.deleted = id
	return nil
}

func (r *recordingAgentMemory) Add(_ context.Context, scope agentmemory.Scope, project, content string, _ time.Time) (agentmemory.Item, error) {
	r.addScope, r.addProject, r.addContent = scope, project, content
	return agentmemory.Item{ID: "mem_new", Scope: scope, Content: content, Origin: agentmemory.OriginUser, Status: agentmemory.StatusActive}, nil
}

func TestAgentMemoryHandlersDisabled(t *testing.T) {
	s := newTestServer(&stubRuntime{})
	s.agentMemory = agentMemoryUnavailable{}
	if _, err := s.ListAgentMemory(context.Background(), protocol.AgentMemoryListRequest{}); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("list err = %v, want capability_not_negotiated", err)
	}
	if err := s.ReviewAgentMemory(context.Background(), protocol.AgentMemoryReviewRequest{ID: "x", Decision: "approve"}); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("review err = %v, want capability_not_negotiated", err)
	}
}

func TestAgentMemoryListResolvesTargetAndMapsWire(t *testing.T) {
	rec := &recordingAgentMemory{items: []agentmemory.Item{
		{ID: "1", Content: "- fact", Origin: agentmemory.OriginAuto, Status: agentmemory.StatusPending},
	}}
	s := newTestServer(&stubRuntime{})
	s.agentMemory = rec

	out, err := s.ListAgentMemory(context.Background(), protocol.AgentMemoryListRequest{Scope: "project", Cwd: "/repo/"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.listScope != agentmemory.ScopeProject || rec.listProject != "/repo" {
		t.Fatalf("target = %v %q, want project /repo", rec.listScope, rec.listProject)
	}
	if len(out.Items) != 1 || out.Items[0].Status != "pending" || out.Items[0].Origin != "auto" {
		t.Fatalf("wire = %+v", out.Items)
	}

	if _, err := s.ListAgentMemory(context.Background(), protocol.AgentMemoryListRequest{Scope: "user", Cwd: "/ignored"}); err != nil {
		t.Fatal(err)
	}
	if rec.listScope != agentmemory.ScopeUser || rec.listProject != "" {
		t.Fatalf("user target = %v %q, want user ''", rec.listScope, rec.listProject)
	}
}

func TestAgentMemoryReviewMapsDecision(t *testing.T) {
	rec := &recordingAgentMemory{}
	s := newTestServer(&stubRuntime{})
	s.agentMemory = rec

	if err := s.ReviewAgentMemory(context.Background(), protocol.AgentMemoryReviewRequest{ID: "a", Decision: "approve"}); err != nil {
		t.Fatal(err)
	}
	if rec.statusID != "a" || rec.status != agentmemory.StatusActive {
		t.Fatalf("approve → %q %v", rec.statusID, rec.status)
	}
	if err := s.ReviewAgentMemory(context.Background(), protocol.AgentMemoryReviewRequest{ID: "b", Decision: "reject"}); err != nil {
		t.Fatal(err)
	}
	if rec.status != agentmemory.StatusRejected {
		t.Fatalf("reject → %v", rec.status)
	}
	if err := s.ReviewAgentMemory(context.Background(), protocol.AgentMemoryReviewRequest{ID: "c", Decision: "bogus"}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("bogus decision → %v, want invalid_params", err)
	}
}

func TestAgentMemoryUpdateAndAdd(t *testing.T) {
	rec := &recordingAgentMemory{getItem: agentmemory.Item{ID: "a", Content: "- edited", Pinned: true, Status: agentmemory.StatusActive}}
	s := newTestServer(&stubRuntime{})
	s.agentMemory = rec

	content := "- edited"
	pinned := true
	out, err := s.UpdateAgentMemory(context.Background(), protocol.AgentMemoryUpdateRequest{ID: "a", Content: &content, Pinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	if rec.editedID != "a" || rec.editedTx != "- edited" || rec.pinnedID != "a" || !rec.pinned {
		t.Fatalf("update recorded %+v", rec)
	}
	if out.ID != "a" || !out.Pinned {
		t.Fatalf("update wire = %+v", out)
	}

	added, err := s.AddAgentMemory(context.Background(), protocol.AgentMemoryAddRequest{Scope: "project", Cwd: "/repo", Content: "- new note"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.addContent != "- new note" || rec.addProject != "/repo" || added.Origin != "user" {
		t.Fatalf("add recorded=%+v wire=%+v", rec, added)
	}
}
