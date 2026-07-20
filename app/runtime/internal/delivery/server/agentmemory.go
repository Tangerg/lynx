package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// agentMemory.* (API.md §7.x) — HITL review of the agent's self-maintained
// memory: proposals wait as pending until the user approves them, and only
// approved memory reaches the prompt or the memory_search tool.

// A disabled build injects agentMemoryUnavailable so agentMemory.* report
// capability_not_negotiated. The server drives the domain's [agentmemory.Management]
// review surface directly.
var errAgentMemoryDisabled = errors.New("agentMemory: disabled")

var _ agentmemory.Management = agentMemoryUnavailable{}

type agentMemoryUnavailable struct{}

func (agentMemoryUnavailable) List(context.Context, agentmemory.Scope, string) ([]agentmemory.Item, error) {
	return nil, errAgentMemoryDisabled
}
func (agentMemoryUnavailable) Get(context.Context, string) (agentmemory.Item, bool, error) {
	return agentmemory.Item{}, false, errAgentMemoryDisabled
}
func (agentMemoryUnavailable) SetStatus(context.Context, string, agentmemory.Status, time.Time) error {
	return errAgentMemoryDisabled
}
func (agentMemoryUnavailable) SetPinned(context.Context, string, bool, time.Time) error {
	return errAgentMemoryDisabled
}
func (agentMemoryUnavailable) UpdateContent(context.Context, string, string, time.Time) error {
	return errAgentMemoryDisabled
}
func (agentMemoryUnavailable) Delete(context.Context, string) error { return errAgentMemoryDisabled }
func (agentMemoryUnavailable) Add(context.Context, agentmemory.Scope, string, string, time.Time) (agentmemory.Item, error) {
	return agentmemory.Item{}, errAgentMemoryDisabled
}

func agentMemoryOrDisabled(store agentmemory.Management) agentmemory.Management {
	if store == nil {
		return agentMemoryUnavailable{}
	}
	return store
}

// ListAgentMemory returns a project's active + pending memory (agentMemory.list).
func (s *Server) ListAgentMemory(ctx context.Context, in protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error) {
	scope, project := agentMemoryTarget(in.Scope, in.Cwd)
	items, err := s.agentMemory.List(ctx, scope, project)
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
	return mapAgentMemoryErr(s.agentMemory.SetStatus(ctx, in.ID, status, time.Now()), "agentMemory.review")
}

// UpdateAgentMemory edits and/or pins an item (agentMemory.update).
func (s *Server) UpdateAgentMemory(ctx context.Context, in protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) {
	now := time.Now()
	if in.Content != nil {
		if err := s.agentMemory.UpdateContent(ctx, in.ID, *in.Content, now); err != nil {
			return nil, mapAgentMemoryErr(err, "agentMemory.update")
		}
	}
	if in.Pinned != nil {
		if err := s.agentMemory.SetPinned(ctx, in.ID, *in.Pinned, now); err != nil {
			return nil, mapAgentMemoryErr(err, "agentMemory.update")
		}
	}
	item, ok, err := s.agentMemory.Get(ctx, in.ID)
	if err != nil {
		return nil, mapAgentMemoryErr(err, "agentMemory.update")
	}
	if !ok {
		return nil, mapAgentMemoryErr(agentmemory.ErrNotFound, "agentMemory.update")
	}
	w := agentMemoryItemToWire(item)
	return &w, nil
}

// DeleteAgentMemory removes an item (agentMemory.delete).
func (s *Server) DeleteAgentMemory(ctx context.Context, in protocol.AgentMemoryItemRequest) error {
	return mapAgentMemoryErr(s.agentMemory.Delete(ctx, in.ID), "agentMemory.delete")
}

// AddAgentMemory stores a user-authored active item (agentMemory.add).
func (s *Server) AddAgentMemory(ctx context.Context, in protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
	scope, project := agentMemoryTarget(in.Scope, in.Cwd)
	item, err := s.agentMemory.Add(ctx, scope, project, in.Content, time.Now())
	if err != nil {
		return nil, mapAgentMemoryErr(err, "agentMemory.add")
	}
	w := agentMemoryItemToWire(item)
	return &w, nil
}

// agentMemoryTarget resolves the wire (scope, cwd) to the stored (scope,
// project) key: the user scope ignores cwd; the project scope cleans it, the
// same form the extractor writes under.
func agentMemoryTarget(scope, cwd string) (agentmemory.Scope, string) {
	if scope == "user" {
		return agentmemory.ScopeUser, ""
	}
	if cwd == "" {
		return agentmemory.ScopeProject, ""
	}
	return agentmemory.ScopeProject, filepath.Clean(cwd)
}

func mapAgentMemoryErr(err error, method string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errAgentMemoryDisabled):
		return capabilityNotNegotiated(method)
	case errors.Is(err, agentmemory.ErrNotFound):
		return fmt.Errorf("%w: no such memory item", protocol.ErrInvalidParams)
	default:
		return err
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
