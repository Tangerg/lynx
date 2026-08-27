package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/app/runtime/embedded"
	"github.com/Tangerg/scope/app/runtime/protocol"

	"github.com/Tangerg/scope/app/cli/internal/agentmemory"
	"github.com/Tangerg/scope/app/cli/internal/workspace"
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

func (a *agentMemoryAdapter) Items(ctx context.Context, target agentmemory.Target) ([]agentmemory.Item, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	request := protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScope(validated.Scope)}
	if validated.Scope == agentmemory.Project {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
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
		if item.Scope != validated.Scope {
			return nil, runtimeContractViolation("list agent memory item %s belongs to %s, want %s", item.ID, item.Scope, validated.Scope)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, runtimeContractViolation("list agent memory repeats %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		items = append(items, item)
	}
	return items, nil
}

func (a *agentMemoryAdapter) Review(ctx context.Context, id string, decision agentmemory.ReviewDecision) error {
	r := a.runtime
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

func (a *agentMemoryAdapter) Update(ctx context.Context, patch agentmemory.Patch) (agentmemory.Item, error) {
	r := a.runtime
	if err := patch.Validate(); err != nil {
		return agentmemory.Item{}, err
	}
	validated := patch
	if patch.Content != nil {
		content := strings.TrimSpace(*patch.Content)
		validated.Content = &content
	}
	options, err := r.commandOptions()
	if err != nil {
		return agentmemory.Item{}, err
	}
	result, err := r.agentMemory.UpdateAgentMemory(ctx, protocol.AgentMemoryUpdateRequest{
		ID: validated.ID, Content: clonePointer(validated.Content), Pinned: clonePointer(validated.Pinned),
	}, options)
	item, err := projectAgentMemoryResult("update agent memory", validated.ID, "", result, err)
	if err != nil {
		return agentmemory.Item{}, err
	}
	if err := validated.ValidateResult(item); err != nil {
		return agentmemory.Item{}, runtimeContractViolation("update agent memory returned an invalid acknowledgement: %v", err)
	}
	return item, nil
}

func (a *agentMemoryAdapter) Delete(ctx context.Context, id string) error {
	r := a.runtime
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

func (a *agentMemoryAdapter) Add(ctx context.Context, target agentmemory.Target, content string) (agentmemory.Item, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return agentmemory.Item{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return agentmemory.Item{}, errors.New("add agent memory: content is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return agentmemory.Item{}, err
	}
	request := protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScope(validated.Scope), Content: content}
	if validated.Scope == agentmemory.Project {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
	}
	result, err := r.agentMemory.AddAgentMemory(ctx, request, options)
	item, err := projectAgentMemoryResult("add agent memory", "", validated.Scope, result, err)
	if err != nil {
		return agentmemory.Item{}, err
	}
	if err := validated.ValidateAddResult(content, item); err != nil {
		return agentmemory.Item{}, runtimeContractViolation("add agent memory returned an invalid acknowledgement: %v", err)
	}
	return item, nil
}

func (a *agentMemoryAdapter) resolveTarget(ctx context.Context, target agentmemory.Target) (agentmemory.Target, error) {
	if err := target.Validate(); err != nil {
		return agentmemory.Target{}, err
	}
	if target.Scope != agentmemory.Project {
		return target, nil
	}
	resolved, err := a.runtime.Resolve(ctx, workspace.ResolveRequest{Path: target.Workspace})
	if err != nil {
		return agentmemory.Target{}, fmt.Errorf("resolve agent memory workspace: %w", err)
	}
	return agentmemory.NewTarget(target.Scope, resolved.Path)
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
