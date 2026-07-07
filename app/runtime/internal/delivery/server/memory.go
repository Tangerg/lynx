package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// ListMemory enumerates LYRA.md entries across scopes (API.md §7.7).
// Empty (not an error) when no memory store is configured, so the UI
// renders an empty state rather than a banner.
func (s *Server) ListMemory(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.MemoryEntry], error) {
	if !s.knowledge.HasMemory() {
		return protocol.NewPage([]protocol.MemoryEntry{}), nil
	}
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	entries, err := s.knowledge.ListMemoryEntries(ctx, root)
	if err != nil {
		return nil, err
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
	if !s.knowledge.HasMemory() {
		return nil, capabilityNotNegotiated("memory.get")
	}
	scope, cwd, err := s.memoryTargetFromWire(in.Scope, in.Cwd)
	if err != nil {
		return nil, err
	}
	content, err := s.knowledge.Memory(ctx, scope, cwd)
	if err != nil {
		return nil, err
	}
	return &protocol.MemoryEntry{Scope: in.Scope, Content: content}, nil
}

func (s *Server) UpdateMemory(ctx context.Context, in protocol.UpdateMemoryRequest) error {
	if !s.knowledge.HasMemory() {
		return capabilityNotNegotiated("memory.update")
	}
	scope, cwd, err := s.memoryTargetFromWire(in.Scope, in.Cwd)
	if err != nil {
		return err
	}
	return s.knowledge.UpdateMemory(ctx, scope, cwd, in.Content)
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
	root, err := s.workspaceRoot(cwd)
	if err != nil {
		return target, "", err
	}
	return target, root, nil
}
