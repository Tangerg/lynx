package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Codebase (API.md §7.10) ────────────────────────────────────────
//
// The @codebase semantic index: search (the @codebase mention), status, and a
// background reindex. The agent reaches the same index via codebase_search.

func (d *Dispatcher) handleCodebaseSearch(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.CodebaseSearchRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Query == "" {
		return responseError(msg.ID, invalidParams("query is required"))
	}
	out, err := d.api.CodebaseSearch(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleCodebaseStatus(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.CodebaseStatusRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.CodebaseStatus(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleCodebaseReindex(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.CodebaseReindexRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	return replyDone(msg, d.api.CodebaseReindex(ctx, in))
}
