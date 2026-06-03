package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Providers / Models / Tools (API.md §7.6) ───────────────────────

func (d *Dispatcher) handleProvidersList(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.ListProviders(ctx)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleProvidersConfigure(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ConfigureProviderRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ProviderID == "" {
		return responseError(msg.ID, invalidParams("providerId is required"))
	}
	out, err := d.api.ConfigureProvider(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleProvidersTest(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "providerId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, tErr := d.api.TestProvider(ctx, id)
	return reply(msg, out, tErr)
}

func (d *Dispatcher) handleModelsList(ctx context.Context, msg *transport.Request) HandleResult {
	providerID, _ := decodeStringParam(msg.Params, "providerId") // optional
	out, err := d.api.ListModels(ctx, providerID)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleToolsList(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.ListTools(ctx)
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

// ─── Attachments (API.md §7.7) ──────────────────────────────────────

func (d *Dispatcher) handleAttachmentsCreateUpload(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.CreateUploadURLRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.CreateUploadURL(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleAttachmentsGet(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "attachmentId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, gErr := d.api.GetAttachment(ctx, id)
	return reply(msg, out, gErr)
}

func (d *Dispatcher) handleAttachmentsDelete(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "attachmentId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	return replyDone(msg, d.api.DeleteAttachment(ctx, id))
}

// ─── Background (API.md §7.7) ───────────────────────────────────────

func (d *Dispatcher) handleBackgroundList(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.ListBackground(ctx)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleBackgroundCancel(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "taskId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	return replyDone(msg, d.api.CancelBackground(ctx, id))
}

// ─── Feedback (API.md §7.7) ─────────────────────────────────────────

func (d *Dispatcher) handleFeedbackCreate(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.FeedbackRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	return replyDone(msg, d.api.CreateFeedback(ctx, in))
}
