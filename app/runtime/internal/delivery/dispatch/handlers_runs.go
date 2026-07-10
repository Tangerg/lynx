package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Lifecycle ──────────────────────────────────────────────────────

func (d *Dispatcher) handleDiscover(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.Discover(ctx)
	return reply(msg, out, err)
}

func (d *Dispatcher) handlePing(ctx context.Context, msg *transport.Request) HandleResult {
	return replyDone(msg, d.api.Ping(ctx))
}

// ─── Runs (API.md §7.3) ─────────────────────────────────────────────

func (d *Dispatcher) handleRunsStart(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.StartRunRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, events, err := d.api.StartRun(ctx, in)
	return replyStream(ctx, msg, out, events, err)
}

// handleRunsResume answers open interrupts by starting a continuation
// run (R model, API.md §6). The continuation is a fresh root stream.
func (d *Dispatcher) handleRunsResume(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ResumeRunRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ParentRunID == "" {
		return responseError(msg.ID, invalidParams("parentRunId is required"))
	}
	out, events, err := d.api.ResumeRun(ctx, in)
	return replyStream(ctx, msg, out, events, err)
}

// handleRunsSubscribe rebinds an existing root run's stream to the
// caller (reconnect / crash recovery; subscribes the whole run tree).
func (d *Dispatcher) handleRunsSubscribe(ctx context.Context, msg *transport.Request) HandleResult {
	runID, err := decodeStringParam(msg.Params, "runId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, events, sErr := d.api.SubscribeRun(ctx, runID)
	return replyStream(ctx, msg, out, events, sErr)
}

func (d *Dispatcher) handleRunsCancel(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.CancelRunRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.RunID == "" {
		return responseError(msg.ID, invalidParams("runId is required"))
	}
	return replyDone(msg, d.api.CancelRun(ctx, in))
}

func (d *Dispatcher) handleRunsSteer(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SteerRunRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.RunID == "" || in.Message == "" {
		return responseError(msg.ID, invalidParams("runId and message are required"))
	}
	return replyDone(msg, d.api.SteerRun(ctx, in))
}

func (d *Dispatcher) handleRunsList(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ListRunsRequest
	_ = unmarshal(msg.Params, &in) // empty params is valid
	runs, err := d.api.ListRuns(ctx, in)
	return reply(msg, runs, err)
}

func (d *Dispatcher) handleRunsListOpenInterrupts(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ListOpenInterruptsRequest
	_ = unmarshal(msg.Params, &in)
	open, err := d.api.ListOpenInterrupts(ctx, in)
	return reply(msg, open, err)
}
