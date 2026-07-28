package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Memory — LYRA.md long-term memory (API.md §7.7) ────────────────

func (d *Dispatcher) handleMemoryList(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceListQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	entries, err := d.api.ListMemory(ctx, in)
	return reply(msg, entries, err)
}

func (d *Dispatcher) handleMemoryGet(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.GetMemoryRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.GetMemory(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleMemoryUpdate(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.UpdateMemoryRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	return replyDone(msg, d.api.UpdateMemory(ctx, in))
}
