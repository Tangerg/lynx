package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Lifecycle ──────────────────────────────────────────────────────

func (d *Dispatcher) handleInitialize(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.InitializeRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, err := d.api.Initialize(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	d.initialized.Store(true)
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handlePing(ctx context.Context, msg *transport.Request) HandleResult {
	if err := d.api.Ping(ctx); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

// ─── Runs ───────────────────────────────────────────────────────────

func (d *Dispatcher) handleRunsStart(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.StartRunRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, events, results, err := d.api.StartRun(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return streamingResult(msg.ID, out, out.RunID, events, results)
}

func (d *Dispatcher) handleRunsCancel(ctx context.Context, msg *transport.Request) HandleResult {
	// API.md v4 §3.5: runs.cancel is a Request (not a notification).
	// It stops an in-flight run identified by runId. Decoupled from
	// notifications/canceled (which aborts an in-flight JSON-RPC
	// Request — different semantic).
	var in protocol.CancelRunRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.RunID == "" {
		return responseError(msg.ID, invalidParams("runId is required"))
	}
	if err := d.api.CancelRun(ctx, in.RunID); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

func (d *Dispatcher) handleRunsApprovalSubmit(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.SubmitApprovalRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.SubmitApproval(ctx, in); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

func (d *Dispatcher) handleRunsQuestionAnswer(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.AnswerQuestionRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.AnswerQuestion(ctx, in); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}
