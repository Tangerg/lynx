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

// agentMemoryUseCases is Delivery's consumer-side port. Its methods express
// complete review use cases, rather than exposing the domain store workflow.
type agentMemoryUseCases interface {
	Available() bool
	List(ctx context.Context, scope agentmemory.Scope, cwd string) ([]agentmemory.Item, error)
	Review(ctx context.Context, id string, status agentmemory.Status) error
	Update(ctx context.Context, id string, content *string, pinned *bool) (agentmemory.Item, error)
	Delete(ctx context.Context, id string) error
	Add(ctx context.Context, scope agentmemory.Scope, cwd, content string) (agentmemory.Item, error)
}

// ListAgentMemory returns active + pending memory for one explicit target
// (agentMemory.list).
func (s *Server) ListAgentMemory(ctx context.Context, in protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error) {
	scope, cwd, err := agentMemoryTargetFromWire(in.Scope, in.Workspace)
	if err != nil {
		return nil, err
	}
	items, err := s.agentMemory.List(ctx, scope, cwd)
	if err != nil {
		return nil, mapAgentMemoryErr(err, "agentMemory.list")
	}
	out := protocol.AgentMemoryList{Items: make([]protocol.AgentMemoryItem, 0, len(items))}
	for _, item := range items {
		wire, err := agentMemoryItemToWire(item)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, wire)
	}
	return &out, nil
}

// ReviewAgentMemory approves or rejects a pending proposal (agentMemory.review).
func (s *Server) ReviewAgentMemory(ctx context.Context, in protocol.AgentMemoryReviewRequest) error {
	var status agentmemory.Status
	switch in.Decision {
	case protocol.AgentMemoryReviewApprove:
		status = agentmemory.StatusActive
	case protocol.AgentMemoryReviewReject:
		status = agentmemory.StatusRejected
	default:
		return fmt.Errorf("%w: decision must be \"approve\" or \"reject\"", protocol.ErrInvalidParams)
	}
	return mapAgentMemoryErr(s.agentMemory.Review(ctx, in.ID, status), "agentMemory.review")
}

// UpdateAgentMemory edits and/or pins an item (agentMemory.update).
func (s *Server) UpdateAgentMemory(ctx context.Context, in protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) {
	item, err := s.agentMemory.Update(ctx, in.ID, in.Content, in.Pinned)
	if err != nil {
		return nil, mapAgentMemoryErr(err, "agentMemory.update")
	}
	w, err := agentMemoryItemToWire(item)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// DeleteAgentMemory removes an item (agentMemory.delete).
func (s *Server) DeleteAgentMemory(ctx context.Context, in protocol.AgentMemoryItemRequest) error {
	return mapAgentMemoryErr(s.agentMemory.Delete(ctx, in.ID), "agentMemory.delete")
}

// AddAgentMemory stores a user-authored active item (agentMemory.add).
func (s *Server) AddAgentMemory(ctx context.Context, in protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
	scope, cwd, err := agentMemoryTargetFromWire(in.Scope, in.Workspace)
	if err != nil {
		return nil, err
	}
	item, err := s.agentMemory.Add(ctx, scope, cwd, in.Content)
	if err != nil {
		return nil, mapAgentMemoryErr(err, "agentMemory.add")
	}
	w, err := agentMemoryItemToWire(item)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// agentMemoryTargetFromWire maps the closed wire target to the Application's
// scope + workspace input. It also protects direct in-process callers: project
// always has a workspace and user never carries one, matching the generated wire
// constraint instead of silently ignoring a half-valid request.
func agentMemoryTargetFromWire(scope protocol.AgentMemoryScope, workspace *protocol.WorkspaceRef) (agentmemory.Scope, string, error) {
	switch scope {
	case protocol.AgentMemoryScopeProject:
		if workspace == nil {
			return 0, "", fmt.Errorf("%w: project agent memory requires workspace", protocol.ErrInvalidParams)
		}
		return agentmemory.ScopeProject, workspace.Path, nil
	case protocol.AgentMemoryScopeUser:
		if workspace != nil {
			return 0, "", fmt.Errorf("%w: user agent memory forbids workspace", protocol.ErrInvalidParams)
		}
		return agentmemory.ScopeUser, "", nil
	default:
		return 0, "", fmt.Errorf("%w: unknown agent memory scope %q", protocol.ErrInvalidParams, scope)
	}
}

func mapAgentMemoryErr(err error, method string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, agentmemoryapp.ErrUnavailable):
		return capabilityNotNegotiated(method)
	case errors.Is(err, agentmemory.ErrNotFound):
		return fmt.Errorf("%w: no such memory item", protocol.ErrInvalidParams)
	default:
		return wireWorkspaceError(err)
	}
}

func agentMemoryItemToWire(item agentmemory.Item) (protocol.AgentMemoryItem, error) {
	scope, err := agentMemoryScopeWire(item.Scope)
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	origin, err := agentMemoryOriginWire(item.Origin)
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	status, err := agentMemoryStatusWire(item.Status)
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	return protocol.AgentMemoryItem{
		ID:        item.ID,
		Scope:     scope,
		Content:   item.Content,
		Origin:    origin,
		Status:    status,
		Pinned:    item.Pinned,
		SessionID: item.SessionID,
		Day:       item.Day,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}, nil
}

func agentMemoryScopeWire(scope agentmemory.Scope) (protocol.AgentMemoryScope, error) {
	switch scope {
	case agentmemory.ScopeProject:
		return protocol.AgentMemoryScopeProject, nil
	case agentmemory.ScopeUser:
		return protocol.AgentMemoryScopeUser, nil
	default:
		return "", fmt.Errorf("agentMemory: unsupported scope %d", scope)
	}
}

func agentMemoryOriginWire(origin agentmemory.Origin) (protocol.AgentMemoryOrigin, error) {
	switch origin {
	case agentmemory.OriginAuto:
		return protocol.AgentMemoryOriginAuto, nil
	case agentmemory.OriginUser:
		return protocol.AgentMemoryOriginUser, nil
	default:
		return "", fmt.Errorf("agentMemory: unsupported origin %d", origin)
	}
}

func agentMemoryStatusWire(status agentmemory.Status) (protocol.AgentMemoryStatus, error) {
	switch status {
	case agentmemory.StatusActive:
		return protocol.AgentMemoryStatusActive, nil
	case agentmemory.StatusPending:
		return protocol.AgentMemoryStatusPending, nil
	case agentmemory.StatusRejected:
		return "", errors.New("agentMemory: rejected items must not be projected")
	default:
		return "", fmt.Errorf("agentMemory: unsupported status %d", status)
	}
}
