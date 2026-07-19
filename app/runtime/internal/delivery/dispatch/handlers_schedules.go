package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Schedules (API.md §7.9) ────────────────────────────────────────
//
// Cron-triggered headless runs of a saved prompt. The scheduler worker fires
// them while the server process is up; these methods manage the set.

func (d *Dispatcher) handleSchedulesList(ctx context.Context, msg *transport.Request) HandleResult {
	query, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListSchedules(ctx, query)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleSchedulesCreate(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.CreateScheduleRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.CreateSchedule(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleSchedulesUpdate(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.UpdateScheduleRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ID == "" {
		return responseError(msg.ID, invalidParams("id is required"))
	}
	if in.ExpectedRevision == 0 {
		return responseError(msg.ID, invalidParams("expectedRevision must be greater than zero"))
	}
	out, err := d.api.UpdateSchedule(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleSchedulesDelete(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.DeleteScheduleRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ID == "" {
		return responseError(msg.ID, invalidParams("id is required"))
	}
	return replyDone(msg, d.api.DeleteSchedule(ctx, in))
}

func (d *Dispatcher) handleSchedulesRunNow(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.RunScheduleNowRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ID == "" {
		return responseError(msg.ID, invalidParams("id is required"))
	}
	out, err := d.api.RunScheduleNow(ctx, in)
	return reply(msg, out, err)
}
