package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Providers / Models / Tools ─────────────────────────────────────

func (d *Dispatcher) handleProvidersList(ctx context.Context, msg *transport.Request) HandleResult {
	providers, err := d.api.ListProviders(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, providers)
}

func (d *Dispatcher) handleProvidersTest(ctx context.Context, msg *transport.Request) HandleResult {
	var in struct {
		ID string `json:"id"`
	}
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	res, err := d.api.TestProvider(ctx, in.ID)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, res)
}

func (d *Dispatcher) handleProvidersConfigure(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.ConfigureProviderRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.ID == "" {
		return responseError(msg.ID, invalidParams("id is required"))
	}
	out, err := d.api.ConfigureProvider(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleModelsList(ctx context.Context, msg *transport.Request) HandleResult {
	var in struct {
		Provider string `json:"provider"`
	}
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	models, err := d.api.ListModels(ctx, in.Provider)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, models)
}

func (d *Dispatcher) handleToolsList(ctx context.Context, msg *transport.Request) HandleResult {
	tools, err := d.api.ListTools(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, tools)
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

// ─── Attachments ────────────────────────────────────────────────────

func (d *Dispatcher) handleAttachmentsCreateUploadURL(ctx context.Context, msg *transport.Request) HandleResult {
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

func (d *Dispatcher) handleAttachmentsDelete(ctx context.Context, msg *transport.Request) HandleResult {
	id, err := decodeIDParam(msg.Params, "id")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.DeleteAttachment(ctx, id); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

// ─── Background ─────────────────────────────────────────────────────

func (d *Dispatcher) handleBackgroundList(ctx context.Context, msg *transport.Request) HandleResult {
	tasks, err := d.api.ListBackground(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, tasks)
}

func (d *Dispatcher) handleBackgroundStop(ctx context.Context, msg *transport.Request) HandleResult {
	// API.md v3 §4.1: param key is `taskId`, not generic `id`.
	taskID, err := decodeIDParam(msg.Params, "taskId")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.StopBackground(ctx, taskID); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

// ─── Feedback ───────────────────────────────────────────────────────

func (d *Dispatcher) handleFeedbackSubmit(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.FeedbackRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.SubmitFeedback(ctx, in); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}
