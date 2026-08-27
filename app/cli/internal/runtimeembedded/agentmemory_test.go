package runtimeembedded

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/embedded"
	"github.com/Tangerg/scope/app/runtime/protocol"

	"github.com/Tangerg/scope/app/cli/internal/agentmemory"
)

type agentMemoryBindingStub struct {
	t            *testing.T
	actions      []string
	now          time.Time
	listed       *protocol.AgentMemoryList
	nilList      bool
	updateResult *protocol.AgentMemoryItem
	addResult    *protocol.AgentMemoryItem
}

func (a *agentMemoryBindingStub) ListAgentMemory(_ context.Context, request protocol.AgentMemoryListRequest, options embedded.CallOptions) (*protocol.AgentMemoryList, error) {
	a.assertMeta(options.RequestMeta)
	switch request.Scope {
	case protocol.AgentMemoryScopeProject:
		if request.Workspace == nil || request.Workspace.Path != "/workspace" {
			a.t.Fatalf("project list request = %+v", request)
		}
	case protocol.AgentMemoryScopeUser:
		if request.Workspace != nil {
			a.t.Fatalf("user list request leaked workspace: %+v", request)
		}
	default:
		a.t.Fatalf("list request = %+v", request)
	}
	if a.nilList {
		return nil, nil
	}
	if a.listed != nil {
		return a.listed, nil
	}
	return &protocol.AgentMemoryList{Items: []protocol.AgentMemoryItem{{
		ID: "mem_1", Scope: request.Scope, Content: "durable fact", Origin: protocol.AgentMemoryOriginAuto,
		Status: protocol.AgentMemoryStatusPending, CreatedAt: a.now, UpdatedAt: a.now,
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
			adapter := &agentMemoryAdapter{runtime: &Runtime{
				agentMemory: stub,
				workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
					Ref:          protocol.WorkspaceRef{Path: "/workspace"},
					ProjectRoot:  "/workspace",
					Availability: protocol.WorkspaceAvailable,
				}},
				meta: requestMeta("test"),
			}}
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

func (a *agentMemoryBindingStub) ReviewAgentMemory(_ context.Context, request protocol.AgentMemoryReviewRequest, options embedded.CommandOptions) error {
	a.assertCommand(options)
	a.actions = append(a.actions, "review:"+request.ID+":"+string(request.Decision))
	return nil
}

func (a *agentMemoryBindingStub) UpdateAgentMemory(_ context.Context, request protocol.AgentMemoryUpdateRequest, options embedded.CommandOptions) (*protocol.AgentMemoryItem, error) {
	a.assertCommand(options)
	if request.Content == nil || *request.Content != "edited" || request.Pinned == nil || !*request.Pinned {
		a.t.Fatalf("update request = %+v", request)
	}
	a.actions = append(a.actions, "update:"+request.ID)
	if a.updateResult != nil {
		return a.updateResult, nil
	}
	return a.item(request.ID, protocol.AgentMemoryScopeProject, "edited", true), nil
}

func (a *agentMemoryBindingStub) DeleteAgentMemory(_ context.Context, request protocol.AgentMemoryItemRequest, options embedded.CommandOptions) error {
	a.assertCommand(options)
	a.actions = append(a.actions, "delete:"+request.ID)
	return nil
}

func (a *agentMemoryBindingStub) AddAgentMemory(_ context.Context, request protocol.AgentMemoryAddRequest, options embedded.CommandOptions) (*protocol.AgentMemoryItem, error) {
	a.assertCommand(options)
	if request.Scope != protocol.AgentMemoryScopeUser || request.Workspace != nil || request.Content != "authored" {
		a.t.Fatalf("add request = %+v", request)
	}
	a.actions = append(a.actions, "add:user")
	if a.addResult != nil {
		return a.addResult, nil
	}
	return a.item("mem_2", request.Scope, request.Content, false), nil
}

func (a *agentMemoryBindingStub) item(id string, scope protocol.AgentMemoryScope, content string, pinned bool) *protocol.AgentMemoryItem {
	return &protocol.AgentMemoryItem{
		ID: id, Scope: scope, Content: content, Origin: protocol.AgentMemoryOriginUser,
		Status: protocol.AgentMemoryStatusActive, Pinned: pinned, CreatedAt: a.now, UpdatedAt: a.now,
	}
}

func (a *agentMemoryBindingStub) assertMeta(meta protocol.RequestMeta) {
	a.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		a.t.Fatalf("request meta = %+v", meta)
	}
}

func (a *agentMemoryBindingStub) assertCommand(options embedded.CommandOptions) {
	a.t.Helper()
	a.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		a.t.Fatal("command has no idempotency key")
	}
}

func TestAgentMemoryAdapterPreservesTargetReviewAndMutationSemantics(t *testing.T) {
	stub := &agentMemoryBindingStub{t: t, now: time.Now()}
	adapter := &agentMemoryAdapter{runtime: &Runtime{
		agentMemory: stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: "/workspace"},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
	}}
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
	if _, itemsErr := adapter.Items(t.Context(), user); itemsErr != nil {
		t.Fatal(itemsErr)
	}
	if reviewErr := adapter.Review(t.Context(), "mem_1", agentmemory.Approve); reviewErr != nil {
		t.Fatal(reviewErr)
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

func TestAgentMemoryAdapterRejectsMutationAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	now := time.Now()
	wrongUpdate := protocol.AgentMemoryItem{
		ID: "mem_1", Scope: protocol.AgentMemoryScopeProject, Content: "ignored",
		Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive, Pinned: true,
		CreatedAt: now, UpdatedAt: now,
	}
	wrongAdd := protocol.AgentMemoryItem{
		ID: "mem_2", Scope: protocol.AgentMemoryScopeUser, Content: "ignored",
		Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	tests := []struct {
		name   string
		stub   *agentMemoryBindingStub
		invoke func(*agentMemoryAdapter) error
	}{
		{
			name: "update content",
			stub: &agentMemoryBindingStub{updateResult: &wrongUpdate},
			invoke: func(adapter *agentMemoryAdapter) error {
				content, pinned := "edited", true
				_, err := adapter.Update(t.Context(), agentmemory.Patch{ID: "mem_1", Content: &content, Pinned: &pinned})
				return err
			},
		},
		{
			name: "add content",
			stub: &agentMemoryBindingStub{addResult: &wrongAdd},
			invoke: func(adapter *agentMemoryAdapter) error {
				target, err := agentmemory.NewTarget(agentmemory.User, "")
				if err != nil {
					return err
				}
				_, err = adapter.Add(t.Context(), target, "authored")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.stub.t, test.stub.now = t, now
			adapter := &agentMemoryAdapter{runtime: &Runtime{agentMemory: test.stub, meta: requestMeta("test")}}
			requireRuntimeContractViolation(t, test.invoke(adapter))
		})
	}
}
