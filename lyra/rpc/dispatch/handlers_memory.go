package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Memory (LYRA.md long-term memory) ──────────────────────────────

func (d *Dispatcher) handleMemoryList(ctx context.Context, msg *transport.Request) HandleResult {
	entries, err := d.api.ListMemory(ctx)
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
		return responseError(msg.ID, invalidParams(`scope must be "project" or "user"`))
	}
	out, err := d.api.GetMemory(ctx, in.Scope)
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
		return responseError(msg.ID, invalidParams(`scope must be "project" or "user"`))
	}
	if err := d.api.UpdateMemory(ctx, in); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}
