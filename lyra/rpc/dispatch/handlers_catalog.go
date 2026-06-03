package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Providers / Models / Tools (API.md §7.6) ───────────────────────

func (d *Dispatcher) handleProvidersList(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.ListProviders(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleProvidersConfigure(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ConfigureProviderRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.ProviderID == "" {
		return responseError(msg.ID, invalidParams("providerId is required"))
	}
	out, err := d.api.ConfigureProvider(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleProvidersTest(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "providerId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, err := d.api.TestProvider(ctx, id)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleModelsList(ctx context.Context, msg *transport.Request) HandleResult {
	providerID, _ := decodeStringParam(msg.Params, "providerId") // optional
	out, err := d.api.ListModels(ctx, providerID)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleToolsList(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.ListTools(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleToolsInvoke(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.InvokeToolRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	out, err := d.api.InvokeTool(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

// ─── Attachments (API.md §7.7) ──────────────────────────────────────

func (d *Dispatcher) handleAttachmentsCreateUpload(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.CreateUploadURLRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, err := d.api.CreateUploadURL(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleAttachmentsGet(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "attachmentId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	out, err := d.api.GetAttachment(ctx, id)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleAttachmentsDelete(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "attachmentId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.DeleteAttachment(ctx, id); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

// ─── Background (API.md §7.7) ───────────────────────────────────────

func (d *Dispatcher) handleBackgroundList(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.ListBackground(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleBackgroundCancel(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeStringParam(msg.Params, "taskId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.CancelBackground(ctx, id); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

// ─── Feedback (API.md §7.7) ─────────────────────────────────────────

func (d *Dispatcher) handleFeedbackCreate(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.FeedbackRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.CreateFeedback(ctx, in); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}
