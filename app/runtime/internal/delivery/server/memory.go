package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// ListMemory enumerates LYRA.md entries across scopes (API.md §7.7).
// The entire memory.* group is capability-gated, so an unwired store is a
// capability error rather than a synthetic empty collection.
func (s *Server) ListMemory(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.MemoryEntry], error) {
	entries, err := s.workspaceKnowledge.ListMemoryEntries(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.MemoryEntry, 0, len(entries))
	for _, e := range entries {
		scope, err := memScopeToWire(e.Scope)
		if err != nil {
			return nil, err
		}
		out = append(out, protocol.MemoryEntry{
			Scope:     scope,
			Content:   e.Content,
			UpdatedAt: e.CapturedAt,
		})
	}
	return protocol.NewPage(out), nil
}

// GetMemory returns one scope's LYRA.md content. Dispatch has already
// validated the scope (MemoryScope.Valid).
func (s *Server) GetMemory(ctx context.Context, in protocol.GetMemoryRequest) (*protocol.MemoryEntry, error) {
	scope, cwd, err := s.memoryTargetFromWire(in.Scope, workspaceRefPath(in.Workspace))
	if err != nil {
		return nil, err
	}
	content, err := s.workspaceKnowledge.Memory(ctx, scope, cwd)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	return &protocol.MemoryEntry{Scope: in.Scope, Content: content}, nil
}

func (s *Server) UpdateMemory(ctx context.Context, in protocol.UpdateMemoryRequest) error {
	scope, cwd, err := s.memoryTargetFromWire(in.Scope, workspaceRefPath(in.Workspace))
	if err != nil {
		return err
	}
	return wireWorkspaceError(s.workspaceKnowledge.UpdateMemory(ctx, scope, cwd, in.Content))
}

// memScopeToWire / memScopeFromWire bridge the protocol and Domain closed
// vocabularies. The wire's cwd + projectRoot both
// fold into the project scope (addressed by the request's cwd);
// home maps to the user scope.
func memScopeToWire(scope knowledge.Scope) (protocol.MemoryScope, error) {
	switch scope {
	case knowledge.ScopeProject:
		return protocol.MemoryScopeCwd, nil
	case knowledge.ScopeUser:
		return protocol.MemoryScopeHome, nil
	default:
		return "", fmt.Errorf("memory: unsupported knowledge scope %q", scope)
	}
}

func memScopeFromWire(scope protocol.MemoryScope) (knowledge.Scope, error) {
	switch scope {
	case protocol.MemoryScopeCwd, protocol.MemoryScopeProjectRoot:
		return knowledge.ScopeProject, nil
	case protocol.MemoryScopeHome:
		return knowledge.ScopeUser, nil
	default:
		return "", fmt.Errorf("%w: unknown memory scope %q", protocol.ErrInvalidParams, scope)
	}
}

func (s *Server) memoryTargetFromWire(scope protocol.MemoryScope, cwd string) (knowledge.Scope, string, error) {
	target, err := memScopeFromWire(scope)
	if err != nil {
		return "", "", err
	}
	if target == knowledge.ScopeUser {
		return target, "", nil
	}
	return target, cwd, nil
}
