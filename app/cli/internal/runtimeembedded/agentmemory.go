package runtimeembedded

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agentmemory"
)

type agentMemoryBinding interface {
	ListAgentMemory(context.Context, protocol.AgentMemoryListRequest, embedded.CallOptions) (*protocol.AgentMemoryList, error)
	ReviewAgentMemory(context.Context, protocol.AgentMemoryReviewRequest, embedded.CommandOptions) error
	UpdateAgentMemory(context.Context, protocol.AgentMemoryUpdateRequest, embedded.CommandOptions) (*protocol.AgentMemoryItem, error)
	DeleteAgentMemory(context.Context, protocol.AgentMemoryItemRequest, embedded.CommandOptions) error
	AddAgentMemory(context.Context, protocol.AgentMemoryAddRequest, embedded.CommandOptions) (*protocol.AgentMemoryItem, error)
}

type agentMemoryAdapter struct{ runtime *Runtime }

var _ agentmemory.Service = (*agentMemoryAdapter)(nil)

func (adapter *agentMemoryAdapter) Items(ctx context.Context, target agentmemory.Target) ([]agentmemory.Item, error) {
	r := adapter.runtime
	if err := target.Validate(); err != nil {
		return nil, err
	}
	request := protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScope(target.Scope)}
	if target.Scope == agentmemory.Project {
		request.Workspace = &protocol.WorkspaceRef{Path: target.Workspace}
	}
	result, err := r.agentMemory.ListAgentMemory(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, runtimeContractViolation("list agent memory returned nil")
	}
	items := make([]agentmemory.Item, 0, len(result.Items))
	seen := make(map[string]struct{}, len(result.Items))
	for index, value := range result.Items {
		item := projectAgentMemoryItem(value)
		if err := item.Validate(); err != nil {
			return nil, runtimeContractViolation("list agent memory item %d is invalid: %v", index+1, err)
		}
		if item.Scope != target.Scope {
			return nil, runtimeContractViolation("list agent memory item %s belongs to %s, want %s", item.ID, item.Scope, target.Scope)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, runtimeContractViolation("list agent memory repeats %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		items = append(items, item)
	}
	return items, nil
}

func (adapter *agentMemoryAdapter) Review(ctx context.Context, id string, decision agentmemory.ReviewDecision) error {
	r := adapter.runtime
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("review agent memory: id is empty")
	}
	if err := decision.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.agentMemory.ReviewAgentMemory(ctx, protocol.AgentMemoryReviewRequest{
		ID: id, Decision: protocol.AgentMemoryReviewDecision(decision),
	}, options))
}

func (adapter *agentMemoryAdapter) Update(ctx context.Context, patch agentmemory.Patch) (agentmemory.Item, error) {
	r := adapter.runtime
	if err := patch.Validate(); err != nil {
		return agentmemory.Item{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return agentmemory.Item{}, err
	}
	result, err := r.agentMemory.UpdateAgentMemory(ctx, protocol.AgentMemoryUpdateRequest{
		ID: patch.ID, Content: clonePointer(patch.Content), Pinned: clonePointer(patch.Pinned),
	}, options)
	return projectAgentMemoryResult("update agent memory", patch.ID, "", result, err)
}

func (adapter *agentMemoryAdapter) Delete(ctx context.Context, id string) error {
	r := adapter.runtime
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("delete agent memory: id is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.agentMemory.DeleteAgentMemory(ctx, protocol.AgentMemoryItemRequest{ID: id}, options))
}

func (adapter *agentMemoryAdapter) Add(ctx context.Context, target agentmemory.Target, content string) (agentmemory.Item, error) {
	r := adapter.runtime
	if err := target.Validate(); err != nil {
		return agentmemory.Item{}, err
	}
	if strings.TrimSpace(content) == "" {
		return agentmemory.Item{}, errors.New("add agent memory: content is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return agentmemory.Item{}, err
	}
	request := protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScope(target.Scope), Content: content}
	if target.Scope == agentmemory.Project {
		request.Workspace = &protocol.WorkspaceRef{Path: target.Workspace}
	}
	result, err := r.agentMemory.AddAgentMemory(ctx, request, options)
	item, err := projectAgentMemoryResult("add agent memory", "", target.Scope, result, err)
	if err != nil {
		return agentmemory.Item{}, err
	}
	if item.Scope != target.Scope {
		return agentmemory.Item{}, runtimeContractViolation("add agent memory returned %s scope, want %s", item.Scope, target.Scope)
	}
	return item, nil
}

func projectAgentMemoryResult(
	operation, expectedID string,
	expectedScope agentmemory.Scope,
	result *protocol.AgentMemoryItem,
	err error,
) (agentmemory.Item, error) {
	if err != nil {
		return agentmemory.Item{}, classifyError(err)
	}
	if result == nil {
		return agentmemory.Item{}, runtimeContractViolation("%s returned nil", operation)
	}
	item := projectAgentMemoryItem(*result)
	if err := item.Validate(); err != nil {
		return agentmemory.Item{}, runtimeContractViolation("%s returned an invalid item: %v", operation, err)
	}
	if expectedID != "" && item.ID != expectedID {
		return agentmemory.Item{}, runtimeContractViolation("%s returned id %q for %q", operation, item.ID, expectedID)
	}
	if expectedScope != "" && item.Scope != expectedScope {
		return agentmemory.Item{}, runtimeContractViolation("%s returned %s scope, want %s", operation, item.Scope, expectedScope)
	}
	return item, nil
}

func projectAgentMemoryItem(value protocol.AgentMemoryItem) agentmemory.Item {
	return agentmemory.Item{
		ID: value.ID, Scope: agentmemory.Scope(value.Scope), Content: value.Content,
		Origin: agentmemory.Origin(value.Origin), Status: agentmemory.Status(value.Status), Pinned: value.Pinned,
		SessionID: value.SessionID, Day: value.Day, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}
