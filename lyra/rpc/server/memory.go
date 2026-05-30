package server

import (
	"context"

	memsvc "github.com/Tangerg/lynx/lyra/internal/service/memory"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ListMemory enumerates LYRA.md entries across scopes. When no memory
// service is configured it returns empty (not an error) so the UI
// renders an empty state rather than a banner.
func (i *Server) ListMemory(ctx context.Context) ([]protocol.MemoryEntry, error) {
	mem := i.rt.Memory()
	if mem == nil {
		return []protocol.MemoryEntry{}, nil
	}
	entries, err := mem.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.MemoryEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, protocol.MemoryEntry{
			Scope:      memScopeToWire(e.Scope),
			Content:    e.Content,
			CapturedAt: e.CapturedAt,
		})
	}
	return out, nil
}

// GetMemory returns the LYRA.md content for one scope. Dispatch has
// already validated the scope (MemoryScope.Valid) before this runs.
func (i *Server) GetMemory(ctx context.Context, scope protocol.MemoryScope) (*protocol.GetMemoryResponse, error) {
	mem := i.rt.Memory()
	if mem == nil {
		return nil, notImpl("memory.get")
	}
	content, err := mem.Get(ctx, memScopeFromWire(scope))
	if err != nil {
		return nil, err
	}
	return &protocol.GetMemoryResponse{Scope: scope, Content: content}, nil
}

func (i *Server) UpdateMemory(ctx context.Context, in protocol.UpdateMemoryRequest) error {
	mem := i.rt.Memory()
	if mem == nil {
		return notImpl("memory.update")
	}
	return mem.Update(ctx, memScopeFromWire(in.Scope), in.Content)
}

// memScopeToWire / memScopeFromWire bridge the protocol string enum and
// the memory service's int Scope.
func memScopeToWire(s memsvc.Scope) protocol.MemoryScope {
	if s == memsvc.ScopeUser {
		return protocol.MemoryScopeUser
	}
	return protocol.MemoryScopeProject
}

func memScopeFromWire(s protocol.MemoryScope) memsvc.Scope {
	if s == protocol.MemoryScopeUser {
		return memsvc.ScopeUser
	}
	return memsvc.ScopeProject
}
