package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// recordingAgentMemory captures application use cases driven by agentMemory.*
// handlers, so Delivery's wire mapping is tested without a persistence port.
type recordingAgentMemory struct {
	listScope agentmemory.Scope
	listCwd   string
	items     []agentmemory.Item

	statusID string
	status   agentmemory.Status
	pinnedID string
	pinned   bool
	editedID string
	editedTx string
	deleted  string

	addScope   agentmemory.Scope
	addCwd     string
	addContent string

	getItem agentmemory.Item
}

func (*recordingAgentMemory) Available() bool { return true }

func (r *recordingAgentMemory) List(_ context.Context, scope agentmemory.Scope, cwd string) ([]agentmemory.Item, error) {
	r.listScope, r.listCwd = scope, cwd
	return r.items, nil
}

func (r *recordingAgentMemory) Review(_ context.Context, id string, status agentmemory.Status) error {
	r.statusID, r.status = id, status
	return nil
}

func (r *recordingAgentMemory) Update(_ context.Context, id string, content *string, pinned *bool) (agentmemory.Item, error) {
	if content != nil {
		r.editedID, r.editedTx = id, *content
	}
	if pinned != nil {
		r.pinnedID, r.pinned = id, *pinned
	}
	return r.getItem, nil
}

func (r *recordingAgentMemory) Delete(_ context.Context, id string) error {
	r.deleted = id
	return nil
}

func (r *recordingAgentMemory) Add(_ context.Context, scope agentmemory.Scope, cwd, content string) (agentmemory.Item, error) {
	r.addScope, r.addCwd, r.addContent = scope, cwd, content
	return agentmemory.Item{ID: "mem_new", Scope: scope, Content: content, Origin: agentmemory.OriginUser, Status: agentmemory.StatusActive}, nil
}

func TestAgentMemoryHandlersDisabled(t *testing.T) {
	s := newTestServer(&stubRuntime{})
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
	s.features.agentMemory = true

	out, err := s.ListAgentMemory(context.Background(), protocol.AgentMemoryListRequest{Scope: "project", Cwd: "/repo/"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.listScope != agentmemory.ScopeProject || rec.listCwd != "/repo/" {
		t.Fatalf("input = %v %q, want project /repo/", rec.listScope, rec.listCwd)
	}
	if len(out.Items) != 1 || out.Items[0].Status != "pending" || out.Items[0].Origin != "auto" {
		t.Fatalf("wire = %+v", out.Items)
	}

	if _, err := s.ListAgentMemory(context.Background(), protocol.AgentMemoryListRequest{Scope: "user", Cwd: "/ignored"}); err != nil {
		t.Fatal(err)
	}
	if rec.listScope != agentmemory.ScopeUser || rec.listCwd != "/ignored" {
		t.Fatalf("user input = %v %q, want user /ignored", rec.listScope, rec.listCwd)
	}
}

func TestAgentMemoryReviewMapsDecision(t *testing.T) {
	rec := &recordingAgentMemory{}
	s := newTestServer(&stubRuntime{})
	s.agentMemory = rec
	s.features.agentMemory = true

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
	s.features.agentMemory = true

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
	if rec.addContent != "- new note" || rec.addCwd != "/repo" || added.Origin != "user" {
		t.Fatalf("add recorded=%+v wire=%+v", rec, added)
	}
}
