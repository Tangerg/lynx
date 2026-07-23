package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// ListMemory enumerates LYRA.md entries across scopes (API.md §7.7).
// The entire memory.* group is capability-gated, so an unwired store is a
// capability error rather than a synthetic empty collection.
func (s *Server) ListMemory(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.MemoryEntry], error) {
	if !s.workspaceKnowledge.HasMemory() {
		return nil, capabilityNotNegotiated("memory.list")
	}
	entries, err := s.workspaceKnowledge.ListMemoryEntries(ctx, in.Cwd)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.MemoryEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, protocol.MemoryEntry{
			Scope:     memScopeToWire(e.Scope),
			Content:   e.Content,
			UpdatedAt: e.CapturedAt,
		})
	}
	return protocol.NewPage(out), nil
}

// GetMemory returns one scope's LYRA.md content. Dispatch has already
// validated the scope (MemoryScope.Valid).
func (s *Server) GetMemory(ctx context.Context, in protocol.GetMemoryRequest) (*protocol.MemoryEntry, error) {
	if !s.workspaceKnowledge.HasMemory() {
		return nil, capabilityNotNegotiated("memory.get")
	}
	scope, cwd, err := s.memoryTargetFromWire(in.Scope, in.Cwd)
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
	if !s.workspaceKnowledge.HasMemory() {
		return capabilityNotNegotiated("memory.update")
	}
	scope, cwd, err := s.memoryTargetFromWire(in.Scope, in.Cwd)
	if err != nil {
		return err
	}
	return wireWorkspaceError(s.workspaceKnowledge.UpdateMemory(ctx, scope, cwd, in.Content))
}

// memScopeToWire / memScopeFromWire bridge the protocol string enum and
// the memory store's int Scope. The wire's cwd + projectRoot both
// fold into the project scope (addressed by the request's cwd);
// home maps to the user scope.
func memScopeToWire(s knowledge.Scope) protocol.MemoryScope {
	if s == knowledge.ScopeUser {
		return protocol.MemoryScopeHome
	}
	return protocol.MemoryScopeCwd
}

func memScopeFromWire(s protocol.MemoryScope) knowledge.Scope {
	if s == protocol.MemoryScopeHome {
		return knowledge.ScopeUser
	}
	return knowledge.ScopeProject
}

func (s *Server) memoryTargetFromWire(scope protocol.MemoryScope, cwd string) (knowledge.Scope, string, error) {
	target := memScopeFromWire(scope)
	if target == knowledge.ScopeUser {
		return target, "", nil
	}
	return target, cwd, nil
}
