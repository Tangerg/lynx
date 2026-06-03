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
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, entries)
}

func (d *Dispatcher) handleMemoryGet(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.GetMemoryRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if !in.Scope.Valid() {
		return responseError(msg.ID, invalidParams(`scope must be "cwd" | "projectRoot" | "home"`))
	}
	out, err := d.api.GetMemory(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleMemoryUpdate(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.UpdateMemoryRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if !in.Scope.Valid() {
		return responseError(msg.ID, invalidParams(`scope must be "cwd" | "projectRoot" | "home"`))
	}
	if err := d.api.UpdateMemory(ctx, in); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}
