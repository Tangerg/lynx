package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Approval (API.md §C.3) ─────────────────────────────────────────
//
// The runtime-global tool-permission stance (not per-session): plan is the
// read-only planning stance the exit_plan_mode tool flips back to execute.

func (d *Dispatcher) handleApprovalGetMode(ctx context.Context, msg *transport.Request) HandleResult {
	if bad := decodeEmpty(msg); bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.GetApprovalMode(ctx)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleApprovalSetMode(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SetApprovalModeRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.SetApprovalMode(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleApprovalListRules(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ListApprovalRulesRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListApprovalRules(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleApprovalForgetRule(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ForgetApprovalRuleRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	return replyDone(msg, d.api.ForgetApprovalRule(ctx, in))
}
