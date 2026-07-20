package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Agent memory (API.md §7.x) ─────────────────────────────────────
//
// HITL review of the agent's self-maintained memory: proposed facts wait as
// pending until the user approves them, and only approved memory reaches the
// prompt or the memory_search tool.

func (d *Dispatcher) handleAgentMemoryList(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.AgentMemoryListRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListAgentMemory(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleAgentMemoryReview(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.AgentMemoryReviewRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ID == "" {
		return responseError(msg.ID, invalidParams("id is required"))
	}
	return replyDone(msg, d.api.ReviewAgentMemory(ctx, in))
}

func (d *Dispatcher) handleAgentMemoryUpdate(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.AgentMemoryUpdateRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ID == "" {
		return responseError(msg.ID, invalidParams("id is required"))
	}
	out, err := d.api.UpdateAgentMemory(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleAgentMemoryDelete(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.AgentMemoryItemRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ID == "" {
		return responseError(msg.ID, invalidParams("id is required"))
	}
	return replyDone(msg, d.api.DeleteAgentMemory(ctx, in))
}

func (d *Dispatcher) handleAgentMemoryAdd(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.AgentMemoryAddRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Content == "" {
		return responseError(msg.ID, invalidParams("content is required"))
	}
	out, err := d.api.AddAgentMemory(ctx, in)
	return reply(msg, out, err)
}
