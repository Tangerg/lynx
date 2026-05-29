package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Sessions ───────────────────────────────────────────────────────

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
	id, err := decodeIDParam(msg.Params, "id")
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
	_ = unmarshal(msg.Params, &in)
	sess, err := d.api.CreateSession(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, sess)
}

func (d *Dispatcher) handleSessionsUpdate(ctx context.Context, msg *transport.Request) HandleResult {
	// UpdateSessionRequest is flat — `id` lives alongside the patch
	// fields. One unmarshal pass covers everything (no inline-tag
	// hack).
	var in protocol.UpdateSessionRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.ID == "" {
		return responseError(msg.ID, invalidParams("id is required"))
	}
	sess, err := d.api.UpdateSession(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, sess)
}

func (d *Dispatcher) handleSessionsDelete(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeIDParam(msg.Params, "id")
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
	out, err := d.api.ExportSession(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

// ─── Messages ───────────────────────────────────────────────────────

func (d *Dispatcher) handleMessagesList(ctx context.Context, msg *transport.Request) HandleResult {
	// Flat shape: sessionId + limit + cursor at the same level.
	var in protocol.ListMessagesRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	page, err := d.api.ListMessages(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, page)
}

func (d *Dispatcher) handleMessagesEdit(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.EditMessageRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, err := d.api.EditMessage(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}
