package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Providers / Models / Tools (API.md §7.6) ───────────────────────

func (d *Dispatcher) handleProvidersList(ctx context.Context, msg *transport.Request) HandleResult {
	var q protocol.PageQuery
	_ = unmarshal(msg.Params, &q)
	out, err := d.api.ListProviders(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleProvidersConfigure(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ConfigureProviderRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Provider == "" {
		return responseError(msg.ID, invalidParams("provider is required"))
	}
	out, err := d.api.ConfigureProvider(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleProvidersTest(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "provider")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, tErr := d.api.TestProvider(ctx, id)
	return reply(msg, out, tErr)
}

func (d *Dispatcher) handleModelsList(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ListModelsRequest
	_ = unmarshal(msg.Params, &in) // provider optional; empty → empty page
	out, err := d.api.ListModels(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleModelsGetUtilityRole(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.GetUtilityRole(ctx)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleModelsSetUtilityRole(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.UtilityRole](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.SetUtilityRole(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleToolsList(ctx context.Context, msg *transport.Request) HandleResult {
	var q protocol.PageQuery
	_ = unmarshal(msg.Params, &q)
	out, err := d.api.ListTools(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleToolsInvoke(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.InvokeToolRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	out, err := d.api.InvokeTool(ctx, in)
	return reply(msg, out, err)
}

// ─── Feedback (API.md §7.7) ─────────────────────────────────────────

func (d *Dispatcher) handleFeedbackCreate(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.FeedbackRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	return replyDone(msg, d.api.CreateFeedback(ctx, in))
}
