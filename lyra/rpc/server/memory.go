package server

import (
	"context"

	memsvc "github.com/Tangerg/lynx/lyra/internal/service/memory"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ListMemory enumerates LYRA.md entries across scopes (API.md §7.7).
// Empty (not an error) when no memory service is configured, so the UI
// renders an empty state rather than a banner.
func (s *Server) ListMemory(ctx context.Context, _ protocol.WorkspaceListQuery) (*protocol.Page[protocol.MemoryEntry], error) {
	mem := s.rt.Memory()
	if mem == nil {
		return protocol.NewPage([]protocol.MemoryEntry{}), nil
	}
	entries, err := mem.List(ctx)
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
	mem := s.rt.Memory()
	if mem == nil {
		return nil, notImpl("memory.get")
	}
	content, err := mem.Get(ctx, memScopeFromWire(in.Scope))
	if err != nil {
		return nil, err
	}
	return &protocol.MemoryEntry{Scope: in.Scope, Content: content}, nil
}

func (s *Server) UpdateMemory(ctx context.Context, in protocol.UpdateMemoryRequest) error {
	mem := s.rt.Memory()
	if mem == nil {
		return notImpl("memory.update")
	}
	return mem.Update(ctx, memScopeFromWire(in.Scope), in.Content)
}

// memScopeToWire / memScopeFromWire bridge the protocol string enum and
// the memory service's int Scope. The service has two backing files —
// project (<cwd>/LYRA.md) and user (~/.lyra/LYRA.md); the wire's
// projectRoot folds into project until per-root memory exists.
func memScopeToWire(s memsvc.Scope) protocol.MemoryScope {
	if s == memsvc.ScopeUser {
		return protocol.MemoryScopeHome
	}
	return protocol.MemoryScopeCwd
}

func memScopeFromWire(s protocol.MemoryScope) memsvc.Scope {
	if s == protocol.MemoryScopeHome {
		return memsvc.ScopeUser
	}
	return memsvc.ScopeProject
}
