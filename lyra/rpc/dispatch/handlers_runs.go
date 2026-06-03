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

// ─── Runs (API.md §7.3) ─────────────────────────────────────────────

func (d *Dispatcher) handleRunsStart(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.StartRunRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, events, err := d.api.StartRun(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return streamingResult(msg.ID, out, out.RunID, events)
}

// handleRunsResume answers open interrupts by starting a continuation
// run (R model, API.md §6). The continuation is a fresh root stream.
func (d *Dispatcher) handleRunsResume(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ResumeRunRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.ParentRunID == "" {
		return responseError(msg.ID, invalidParams("parentRunId is required"))
	}
	out, events, err := d.api.ResumeRun(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return streamingResult(msg.ID, out, out.RunID, events)
}

// handleRunsSubscribe rebinds an existing root run's stream to the
// caller (reconnect / crash recovery; subscribes the whole run tree).
func (d *Dispatcher) handleRunsSubscribe(ctx context.Context, msg *transport.Request) HandleResult {
	runID, err := decodeStringParam(msg.Params, "runId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, events, err := d.api.SubscribeRun(ctx, runID)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return streamingResult(msg.ID, out, out.RunID, events)
}

func (d *Dispatcher) handleRunsCancel(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.CancelRunRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.RunID == "" {
		return responseError(msg.ID, invalidParams("runId is required"))
	}
	if err := d.api.CancelRun(ctx, in); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

func (d *Dispatcher) handleRunsList(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ListRunsRequest
	_ = unmarshal(msg.Params, &in) // empty params is valid
	runs, err := d.api.ListRuns(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, runs)
}

func (d *Dispatcher) handleRunsListOpenInterrupts(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ListOpenInterruptsRequest
	_ = unmarshal(msg.Params, &in)
	open, err := d.api.ListOpenInterrupts(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, open)
}
