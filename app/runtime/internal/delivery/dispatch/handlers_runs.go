package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Lifecycle ──────────────────────────────────────────────────────

func (d *Dispatcher) handleDiscover(ctx context.Context, msg *transport.Request) HandleResult {
	if bad := decodeEmpty(msg); bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.Discover(ctx)
	return reply(msg, out, err)
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
	if in.RunID == "" {
		return responseError(msg.ID, invalidParams("runId is required"))
	}
	out, events, err := d.api.ResumeRun(ctx, in)
	return replyStream(ctx, msg, out, events, err)
}

// handleRunsSubscribe rebinds an existing root run's stream to the
// caller (reconnect / crash recovery; subscribes the whole run tree).
func (d *Dispatcher) handleRunsSubscribe(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SubscribeRunRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.RunID == "" {
		return responseError(msg.ID, invalidParams("runId is required"))
	}
	out, events, sErr := d.api.SubscribeRun(ctx, in.RunID)
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
	in, bad := decode[protocol.ListRunsRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	runs, err := d.api.ListRuns(ctx, in)
	return reply(msg, runs, err)
}

func (d *Dispatcher) handleRunsListOpenInterrupts(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ListOpenInterruptsRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	open, err := d.api.ListOpenInterrupts(ctx, in)
	return reply(msg, open, err)
}
