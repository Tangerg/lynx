package runtimeembedded

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agentmemory"
)

type agentMemoryBindingStub struct {
	t       *testing.T
	actions []string
	now     time.Time
	listed  *protocol.AgentMemoryList
	nilList bool
}

func (stub *agentMemoryBindingStub) ListAgentMemory(_ context.Context, request protocol.AgentMemoryListRequest, options embedded.CallOptions) (*protocol.AgentMemoryList, error) {
	stub.assertMeta(options.RequestMeta)
	switch request.Scope {
	case protocol.AgentMemoryScopeProject:
		if request.Workspace == nil || request.Workspace.Path != "/workspace" {
			stub.t.Fatalf("project list request = %+v", request)
		}
	case protocol.AgentMemoryScopeUser:
		if request.Workspace != nil {
			stub.t.Fatalf("user list request leaked workspace: %+v", request)
		}
	default:
		stub.t.Fatalf("list request = %+v", request)
	}
	if stub.nilList {
		return nil, nil
	}
	if stub.listed != nil {
		return stub.listed, nil
	}
	return &protocol.AgentMemoryList{Items: []protocol.AgentMemoryItem{{
		ID: "mem_1", Scope: request.Scope, Content: "durable fact", Origin: protocol.AgentMemoryOriginAuto,
		Status: protocol.AgentMemoryStatusPending, CreatedAt: stub.now, UpdatedAt: stub.now,
	}}}, nil
}

func TestAgentMemoryAdapterRejectsBrokenRuntimeProjections(t *testing.T) {
	now := time.Now()
	valid := protocol.AgentMemoryItem{
		ID: "mem_1", Scope: protocol.AgentMemoryScopeProject, Content: "fact",
		Origin: protocol.AgentMemoryOriginAuto, Status: protocol.AgentMemoryStatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	for _, test := range []struct {
		name    string
		listed  *protocol.AgentMemoryList
		nilList bool
	}{
		{name: "nil list", nilList: true},
		{name: "wrong scope", listed: &protocol.AgentMemoryList{Items: []protocol.AgentMemoryItem{{
			ID: "mem_1", Scope: protocol.AgentMemoryScopeUser, Content: "fact",
			Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive,
			CreatedAt: now, UpdatedAt: now,
		}}}},
		{name: "duplicate identity", listed: &protocol.AgentMemoryList{Items: []protocol.AgentMemoryItem{valid, valid}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &agentMemoryBindingStub{t: t, now: now, listed: test.listed, nilList: test.nilList}
			adapter := &agentMemoryAdapter{runtime: &Runtime{agentMemory: stub, meta: requestMeta("test")}}
			target, err := agentmemory.NewTarget(agentmemory.Project, "/workspace")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.Items(t.Context(), target); err == nil {
				t.Fatal("broken projection was accepted")
			} else {
				requireRuntimeContractViolation(t, err)
			}
		})
	}
}

func (stub *agentMemoryBindingStub) ReviewAgentMemory(_ context.Context, request protocol.AgentMemoryReviewRequest, options embedded.CommandOptions) error {
	stub.assertCommand(options)
	stub.actions = append(stub.actions, "review:"+request.ID+":"+string(request.Decision))
	return nil
}

func (stub *agentMemoryBindingStub) UpdateAgentMemory(_ context.Context, request protocol.AgentMemoryUpdateRequest, options embedded.CommandOptions) (*protocol.AgentMemoryItem, error) {
	stub.assertCommand(options)
	if request.Content == nil || *request.Content != "edited" || request.Pinned == nil || !*request.Pinned {
		stub.t.Fatalf("update request = %+v", request)
	}
	stub.actions = append(stub.actions, "update:"+request.ID)
	return stub.item(request.ID, protocol.AgentMemoryScopeProject, "edited", true), nil
}

func (stub *agentMemoryBindingStub) DeleteAgentMemory(_ context.Context, request protocol.AgentMemoryItemRequest, options embedded.CommandOptions) error {
	stub.assertCommand(options)
	stub.actions = append(stub.actions, "delete:"+request.ID)
	return nil
}

func (stub *agentMemoryBindingStub) AddAgentMemory(_ context.Context, request protocol.AgentMemoryAddRequest, options embedded.CommandOptions) (*protocol.AgentMemoryItem, error) {
	stub.assertCommand(options)
	if request.Scope != protocol.AgentMemoryScopeUser || request.Workspace != nil || request.Content != "authored" {
		stub.t.Fatalf("add request = %+v", request)
	}
	stub.actions = append(stub.actions, "add:user")
	return stub.item("mem_2", request.Scope, request.Content, false), nil
}

func (stub *agentMemoryBindingStub) item(id string, scope protocol.AgentMemoryScope, content string, pinned bool) *protocol.AgentMemoryItem {
	return &protocol.AgentMemoryItem{
		ID: id, Scope: scope, Content: content, Origin: protocol.AgentMemoryOriginUser,
		Status: protocol.AgentMemoryStatusActive, Pinned: pinned, CreatedAt: stub.now, UpdatedAt: stub.now,
	}
}

func (stub *agentMemoryBindingStub) assertMeta(meta protocol.RequestMeta) {
	stub.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		stub.t.Fatalf("request meta = %+v", meta)
	}
}

func (stub *agentMemoryBindingStub) assertCommand(options embedded.CommandOptions) {
	stub.t.Helper()
	stub.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		stub.t.Fatal("command has no idempotency key")
	}
}

func TestAgentMemoryAdapterPreservesTargetReviewAndMutationSemantics(t *testing.T) {
	stub := &agentMemoryBindingStub{t: t, now: time.Now()}
	adapter := &agentMemoryAdapter{runtime: &Runtime{agentMemory: stub, meta: requestMeta("test")}}
	project, err := agentmemory.NewTarget(agentmemory.Project, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	items, err := adapter.Items(t.Context(), project)
	if err != nil || len(items) != 1 || items[0].Status != agentmemory.Pending {
		t.Fatalf("Items = (%+v, %v)", items, err)
	}
	user, err := agentmemory.NewTarget(agentmemory.User, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Items(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Review(t.Context(), "mem_1", agentmemory.Approve); err != nil {
		t.Fatal(err)
	}
	content, pinned := "edited", true
	updated, err := adapter.Update(t.Context(), agentmemory.Patch{ID: "mem_1", Content: &content, Pinned: &pinned})
	if err != nil || updated.Content != content || !updated.Pinned {
		t.Fatalf("Update = (%+v, %v)", updated, err)
	}
	if _, err := adapter.Add(t.Context(), user, "authored"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Delete(t.Context(), "mem_1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"review:mem_1:approve", "update:mem_1", "add:user", "delete:mem_1"}
	if len(stub.actions) != len(want) {
		t.Fatalf("actions = %v, want %v", stub.actions, want)
	}
	for index := range want {
		if stub.actions[index] != want[index] {
			t.Fatalf("actions = %v, want %v", stub.actions, want)
		}
	}
}

func TestAgentMemoryMutationRejectsIdentityDrift(t *testing.T) {
	t.Parallel()
	now := time.Now()
	result := protocol.AgentMemoryItem{
		ID: "mem_other", Scope: protocol.AgentMemoryScopeProject, Content: "edited",
		Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err := projectAgentMemoryResult("update agent memory", "mem_1", "", &result, nil)
	requireRuntimeContractViolation(t, err)
}
