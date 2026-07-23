package server

import (
	"context"
	"errors"
	"fmt"

	agentmemoryapp "github.com/Tangerg/lynx/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// agentMemory.* (API.md §7.x) — HITL review of the agent's self-maintained
// memory: proposals wait as pending until the user approves them, and only
// approved memory reaches the prompt or the memory_search tool.

// Delivery consumes application use cases. It holds neither a persistence port
// nor a disabled fake: nil simply means the capability was not negotiated.
var errAgentMemoryDisabled = errors.New("agentMemory: disabled")

// agentMemoryUseCases is Delivery's consumer-side port. Its methods express
// complete review use cases, rather than exposing the domain store workflow.
type agentMemoryUseCases interface {
	List(ctx context.Context, scope agentmemory.Scope, cwd string) ([]agentmemory.Item, error)
	Review(ctx context.Context, id string, status agentmemory.Status) error
	Update(ctx context.Context, id string, content *string, pinned *bool) (agentmemory.Item, error)
	Delete(ctx context.Context, id string) error
	Add(ctx context.Context, scope agentmemory.Scope, cwd, content string) (agentmemory.Item, error)
}

// ListAgentMemory returns a project's active + pending memory (agentMemory.list).
func (s *Server) ListAgentMemory(ctx context.Context, in protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error) {
	if s.agentMemory == nil {
		return nil, mapAgentMemoryErr(errAgentMemoryDisabled, "agentMemory.list")
	}
	items, err := s.agentMemory.List(ctx, agentMemoryScope(in.Scope), in.Cwd)
	if err != nil {
		return nil, mapAgentMemoryErr(err, "agentMemory.list")
	}
	out := protocol.AgentMemoryList{Items: make([]protocol.AgentMemoryItem, 0, len(items))}
	for _, item := range items {
		out.Items = append(out.Items, agentMemoryItemToWire(item))
	}
	return &out, nil
}

// ReviewAgentMemory approves or rejects a pending proposal (agentMemory.review).
func (s *Server) ReviewAgentMemory(ctx context.Context, in protocol.AgentMemoryReviewRequest) error {
	var status agentmemory.Status
	switch in.Decision {
	case "approve":
		status = agentmemory.StatusActive
	case "reject":
		status = agentmemory.StatusRejected
	default:
		return fmt.Errorf("%w: decision must be \"approve\" or \"reject\"", protocol.ErrInvalidParams)
	}
	if s.agentMemory == nil {
		return mapAgentMemoryErr(errAgentMemoryDisabled, "agentMemory.review")
	}
	return mapAgentMemoryErr(s.agentMemory.Review(ctx, in.ID, status), "agentMemory.review")
}

// UpdateAgentMemory edits and/or pins an item (agentMemory.update).
func (s *Server) UpdateAgentMemory(ctx context.Context, in protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) {
	if s.agentMemory == nil {
		return nil, mapAgentMemoryErr(errAgentMemoryDisabled, "agentMemory.update")
	}
	item, err := s.agentMemory.Update(ctx, in.ID, in.Content, in.Pinned)
	if err != nil {
		return nil, mapAgentMemoryErr(err, "agentMemory.update")
	}
	w := agentMemoryItemToWire(item)
	return &w, nil
}

// DeleteAgentMemory removes an item (agentMemory.delete).
func (s *Server) DeleteAgentMemory(ctx context.Context, in protocol.AgentMemoryItemRequest) error {
	if s.agentMemory == nil {
		return mapAgentMemoryErr(errAgentMemoryDisabled, "agentMemory.delete")
	}
	return mapAgentMemoryErr(s.agentMemory.Delete(ctx, in.ID), "agentMemory.delete")
}

// AddAgentMemory stores a user-authored active item (agentMemory.add).
func (s *Server) AddAgentMemory(ctx context.Context, in protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
	if s.agentMemory == nil {
		return nil, mapAgentMemoryErr(errAgentMemoryDisabled, "agentMemory.add")
	}
	item, err := s.agentMemory.Add(ctx, agentMemoryScope(in.Scope), in.Cwd, in.Content)
	if err != nil {
		return nil, mapAgentMemoryErr(err, "agentMemory.add")
	}
	w := agentMemoryItemToWire(item)
	return &w, nil
}

// agentMemoryScope maps the wire vocabulary to the domain's bounded scope.
// Project-root resolution happens inside the Application use case.
func agentMemoryScope(scope string) agentmemory.Scope {
	if scope == "user" {
		return agentmemory.ScopeUser
	}
	return agentmemory.ScopeProject
}

func mapAgentMemoryErr(err error, method string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errAgentMemoryDisabled):
		return capabilityNotNegotiated(method)
	case errors.Is(err, agentmemoryapp.ErrUnavailable):
		return capabilityNotNegotiated(method)
	case errors.Is(err, agentmemory.ErrNotFound):
		return fmt.Errorf("%w: no such memory item", protocol.ErrInvalidParams)
	default:
		return wireWorkspaceError(err)
	}
}

func agentMemoryItemToWire(item agentmemory.Item) protocol.AgentMemoryItem {
	return protocol.AgentMemoryItem{
		ID:        item.ID,
		Scope:     item.Scope.String(),
		Content:   item.Content,
		Origin:    item.Origin.String(),
		Status:    item.Status.String(),
		Pinned:    item.Pinned,
		SessionID: item.SessionID,
		Day:       item.Day,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}
