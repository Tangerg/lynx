package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Providers / Models / Tools (API.md §7.6) ───────────────────────

func (d *Dispatcher) handleProvidersList(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
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
	in, bad := decode[protocol.TestProviderRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Provider == "" {
		return responseError(msg.ID, invalidParams("provider is required"))
	}
	out, tErr := d.api.TestProvider(ctx, in.Provider)
	return reply(msg, out, tErr)
}

func (d *Dispatcher) handleModelsList(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ListModelsRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListModels(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleModelsGetUtilityRole(ctx context.Context, msg *transport.Request) HandleResult {
	if bad := decodeEmpty(msg); bad != nil {
		return responseError(msg.ID, bad)
	}
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

func (d *Dispatcher) handleModelsGetEmbeddingRole(ctx context.Context, msg *transport.Request) HandleResult {
	if bad := decodeEmpty(msg); bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.GetEmbeddingRole(ctx)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleModelsSetEmbeddingRole(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.EmbeddingRole](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.SetEmbeddingRole(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleToolsList(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
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

// ─── Usage reporting (API.md §7.7) ──────────────────────────────────

func (d *Dispatcher) handleUsageSession(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SessionUsageRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.SessionID == "" {
		return responseError(msg.ID, invalidParams("sessionId is required"))
	}
	out, uErr := d.api.SessionUsage(ctx, in.SessionID)
	return reply(msg, out, uErr)
}

func (d *Dispatcher) handleUsageSummary(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.UsageSummaryRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.UsageSummary(ctx, in)
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
