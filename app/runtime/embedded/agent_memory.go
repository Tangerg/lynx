package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListAgentMemory returns curated Agent memory and review candidates.
func (r *Runtime) ListAgentMemory(ctx context.Context, request protocol.AgentMemoryListRequest, options CallOptions) (*protocol.AgentMemoryList, error) {
	return invoke[protocol.AgentMemoryListRequest, *protocol.AgentMemoryList](ctx, r, "agentMemory.list", request, callOptions(options))
}

// ReviewAgentMemory accepts or rejects an Agent memory candidate.
func (r *Runtime) ReviewAgentMemory(ctx context.Context, request protocol.AgentMemoryReviewRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "agentMemory.review", request, commandOptions(options))
}

// UpdateAgentMemory updates one curated Agent memory item.
func (r *Runtime) UpdateAgentMemory(ctx context.Context, request protocol.AgentMemoryUpdateRequest, options CommandOptions) (*protocol.AgentMemoryItem, error) {
	return invoke[protocol.AgentMemoryUpdateRequest, *protocol.AgentMemoryItem](ctx, r, "agentMemory.update", request, commandOptions(options))
}

// DeleteAgentMemory deletes one curated Agent memory item.
func (r *Runtime) DeleteAgentMemory(ctx context.Context, request protocol.AgentMemoryItemRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "agentMemory.delete", request, commandOptions(options))
}

// AddAgentMemory adds one curated Agent memory item.
func (r *Runtime) AddAgentMemory(ctx context.Context, request protocol.AgentMemoryAddRequest, options CommandOptions) (*protocol.AgentMemoryItem, error) {
	return invoke[protocol.AgentMemoryAddRequest, *protocol.AgentMemoryItem](ctx, r, "agentMemory.add", request, commandOptions(options))
}
