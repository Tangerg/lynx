package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// ListMemory enumerates LYRA.md entries across scopes (API.md §7.7).
// Empty (not an error) when no memory service is configured, so the UI
// renders an empty state rather than a banner.
func (s *Server) ListMemory(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.MemoryEntry], error) {
	if !s.knowledge.HasMemory() {
		return protocol.NewPage([]protocol.MemoryEntry{}), nil
	}
	// in.Cwd scopes the project entry to that directory's LYRA.md;
	// empty keeps the workspace convention "default = serve directory"
	// (the memory service's default dir).
	entries, err := s.knowledge.ListMemoryEntries(ctx, in.Cwd)
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
		return nil, notImpl("memory.get")
	}
	content, err := s.knowledge.GetMemory(ctx, memScopeFromWire(in.Scope), in.Cwd)
	if err != nil {
		return nil, err
	}
	return &protocol.MemoryEntry{Scope: in.Scope, Content: content}, nil
}

func (s *Server) UpdateMemory(ctx context.Context, in protocol.UpdateMemoryRequest) error {
	if !s.knowledge.HasMemory() {
		return notImpl("memory.update")
	}
	return s.knowledge.UpdateMemory(ctx, memScopeFromWire(in.Scope), in.Cwd, in.Content)
}

// memScopeToWire / memScopeFromWire bridge the protocol string enum and
// the memory service's int Scope. The wire's cwd + projectRoot both
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
