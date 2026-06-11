package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Sessions (API.md §7.2) ─────────────────────────────────────────

func (d *Dispatcher) handleSessionsList(ctx context.Context, msg *transport.Request) HandleResult {
	var q protocol.PageQuery
	_ = unmarshal(msg.Params, &q) // empty params is valid
	page, err := d.api.ListSessions(ctx, q)
	return reply(msg, page, err)
}

func (d *Dispatcher) handleSessionsGet(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "sessionId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	sess, gErr := d.api.GetSession(ctx, id)
	return reply(msg, sess, gErr)
}

func (d *Dispatcher) handleSessionsCreate(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.CreateSessionRequest
	_ = unmarshal(msg.Params, &in) // empty body defaults cwd to serve dir
	sess, err := d.api.CreateSession(ctx, in)
	return reply(msg, sess, err)
}

func (d *Dispatcher) handleSessionsUpdate(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.UpdateSessionRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	sess, err := d.api.UpdateSession(ctx, in)
	return reply(msg, sess, err)
}

func (d *Dispatcher) handleSessionsDelete(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "sessionId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	return replyDone(msg, d.api.DeleteSession(ctx, id))
}

func (d *Dispatcher) handleSessionsFork(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ForkSessionRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	sess, err := d.api.ForkSession(ctx, in)
	return reply(msg, sess, err)
}

func (d *Dispatcher) handleSessionsRollback(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.RollbackSessionRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	out, err := d.api.RollbackSession(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleSessionsExport(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ExportSessionRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	out, err := d.api.ExportSession(ctx, in)
	return reply(msg, out, err)
}

// ─── Items (API.md §7.4) ────────────────────────────────────────────

func (d *Dispatcher) handleItemsList(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ListItemsRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	out, err := d.api.ListItems(ctx, in)
	return reply(msg, out, err)
}
