package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Goals (API.md §7.14) ───────────────────────────────────────────
//
// Goal mode: an autonomous loop that drives runs toward an objective until the
// model signals complete/blocked, a budget is spent, or the user stops it.

func (d *Dispatcher) handleGoalsStart(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.StartGoalRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	if in.Objective == "" {
		return responseError(msg.ID, invalidParams("objective is required"))
	}
	out, err := d.api.StartGoal(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleGoalsGet(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.GoalRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	out, err := d.api.GetGoal(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleGoalsStop(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.GoalRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	out, err := d.api.StopGoal(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleGoalsResume(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.GoalRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	out, err := d.api.ResumeGoal(ctx, in)
	return reply(msg, out, err)
}
