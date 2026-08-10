package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// recordingAgentMemory captures application use cases driven by agentMemory.*
// handlers, so Delivery's wire mapping is tested without a persistence port.
type recordingAgentMemory struct {
	listScope agentmemory.Scope
	listCWD   string
	items     []agentmemory.Item

	reviewID string
	decision agentmemory.ReviewDecision
	pinnedID string
	pinned   bool
	editedID string
	editedTx string
	deleted  string

	addScope   agentmemory.Scope
	addCWD     string
	addContent string

	getItem agentmemory.Item
}

func (*recordingAgentMemory) Available() bool { return true }

func (r *recordingAgentMemory) List(_ context.Context, scope agentmemory.Scope, cwd string) ([]agentmemory.Item, error) {
	r.listScope, r.listCWD = scope, cwd
	return r.items, nil
}

func (r *recordingAgentMemory) Review(_ context.Context, id string, decision agentmemory.ReviewDecision) error {
	r.reviewID, r.decision = id, decision
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
	r.addScope, r.addCWD, r.addContent = scope, cwd, content
	return agentmemory.Item{ID: "mem_new", Scope: scope, Content: content, Origin: agentmemory.OriginUser, Status: agentmemory.StatusActive}, nil
}

func TestAgentMemoryListResolvesTargetAndMapsWire(t *testing.T) {
	rec := &recordingAgentMemory{items: []agentmemory.Item{
		{ID: "1", Scope: agentmemory.ScopeProject, Content: "- fact", Origin: agentmemory.OriginAuto, Status: agentmemory.StatusPending},
	}}
	s := newTestServer(&stubRuntime{})
	s.agentMemory = rec
	s.features.agentMemory = true

	out, err := s.ListAgentMemory(context.Background(), protocol.AgentMemoryListRequest{
		Scope: "project", Workspace: &protocol.WorkspaceRef{Path: "/repo/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.listScope != agentmemory.ScopeProject || rec.listCWD != "/repo/" {
		t.Fatalf("input = %v %q, want project /repo/", rec.listScope, rec.listCWD)
	}
	if len(out.Items) != 1 || out.Items[0].Status != "pending" || out.Items[0].Origin != "auto" {
		t.Fatalf("wire = %+v", out.Items)
	}

	if _, err := s.ListAgentMemory(context.Background(), protocol.AgentMemoryListRequest{
		Scope: "user",
	}); err != nil {
		t.Fatal(err)
	}
	if rec.listScope != agentmemory.ScopeUser || rec.listCWD != "" {
		t.Fatalf("user input = %v %q, want user with no workspace", rec.listScope, rec.listCWD)
	}
}

func TestAgentMemoryTargetRefusesPartialTargets(t *testing.T) {
	t.Parallel()

	for _, request := range []protocol.AgentMemoryListRequest{
		{Scope: protocol.AgentMemoryScopeProject},
		{Scope: protocol.AgentMemoryScopeUser, Workspace: &protocol.WorkspaceRef{Path: "/ignored"}},
	} {
		server := newTestServer(&stubRuntime{})
		server.agentMemory = &recordingAgentMemory{}
		if _, err := server.ListAgentMemory(context.Background(), request); !errors.Is(err, protocol.ErrInvalidParams) {
			t.Errorf("ListAgentMemory(%+v) error = %v, want invalid_params", request, err)
		}
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
	if rec.reviewID != "a" || rec.decision != agentmemory.ReviewApprove {
		t.Fatalf("approve → %q %v", rec.reviewID, rec.decision)
	}
	if err := s.ReviewAgentMemory(context.Background(), protocol.AgentMemoryReviewRequest{ID: "b", Decision: "reject"}); err != nil {
		t.Fatal(err)
	}
	if rec.decision != agentmemory.ReviewReject {
		t.Fatalf("reject → %v", rec.decision)
	}
	if err := s.ReviewAgentMemory(context.Background(), protocol.AgentMemoryReviewRequest{ID: "c", Decision: "bogus"}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("bogus decision → %v, want invalid_params", err)
	}
}

func TestAgentMemoryUpdateAndAdd(t *testing.T) {
	rec := &recordingAgentMemory{getItem: agentmemory.Item{
		ID: "a", Scope: agentmemory.ScopeProject, Content: "- edited", Origin: agentmemory.OriginUser,
		Pinned: true, Status: agentmemory.StatusActive,
	}}
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

	added, err := s.AddAgentMemory(context.Background(), protocol.AgentMemoryAddRequest{
		Scope: "project", Workspace: &protocol.WorkspaceRef{Path: "/repo"}, Content: "- new note",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.addContent != "- new note" || rec.addCWD != "/repo" || added.Origin != "user" {
		t.Fatalf("add recorded=%+v wire=%+v", rec, added)
	}
}
