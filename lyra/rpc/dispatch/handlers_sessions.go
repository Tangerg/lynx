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
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, page)
}

func (d *Dispatcher) handleSessionsGet(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "sessionId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	sess, err := d.api.GetSession(ctx, id)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, sess)
}

func (d *Dispatcher) handleSessionsCreate(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.CreateSessionRequest
	_ = unmarshal(msg.Params, &in) // empty body defaults cwd to serve dir
	sess, err := d.api.CreateSession(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, sess)
}

func (d *Dispatcher) handleSessionsUpdate(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.UpdateSessionRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	sess, err := d.api.UpdateSession(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, sess)
}

func (d *Dispatcher) handleSessionsDelete(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "sessionId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.DeleteSession(ctx, id); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

func (d *Dispatcher) handleSessionsFork(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ForkSessionRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	sess, err := d.api.ForkSession(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, sess)
}

func (d *Dispatcher) handleSessionsExport(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ExportSessionRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	out, err := d.api.ExportSession(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

// ─── Items (API.md §7.4) ────────────────────────────────────────────

func (d *Dispatcher) handleItemsList(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ListItemsRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	out, err := d.api.ListItems(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleItemsEdit(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.EditItemRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.ItemID == "" {
		return responseError(msg.ID, invalidParams("itemId is required"))
	}
	out, err := d.api.EditItem(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}
