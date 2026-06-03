package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Memory — LYRA.md long-term memory (API.md §7.7) ────────────────

func (d *Dispatcher) handleMemoryList(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.WorkspaceQuery
	_ = unmarshal(msg.Params, &in)
	entries, err := d.api.ListMemory(ctx, in)
	return reply(msg, entries, err)
}

func (d *Dispatcher) handleMemoryGet(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.GetMemoryRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if !in.Scope.Valid() {
		return responseError(msg.ID, invalidParams(`scope must be "cwd" | "projectRoot" | "home"`))
	}
	out, err := d.api.GetMemory(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleMemoryUpdate(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.UpdateMemoryRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if !in.Scope.Valid() {
		return responseError(msg.ID, invalidParams(`scope must be "cwd" | "projectRoot" | "home"`))
	}
	return replyDone(msg, d.api.UpdateMemory(ctx, in))
}
