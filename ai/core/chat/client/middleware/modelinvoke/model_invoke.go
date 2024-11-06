package modelinvoke

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

type invoker[O request.ChatRequestOptions, M result.ChatResultMetadata] struct{}

func (i *invoker[O, M]) buildChatRequest(req *middleware.Request[O, M]) *request.ChatRequest[O] {
	if req.UserText != "" || req.UserMedia != nil {
		req.AddUserMessage(req.UserText, req.UserParams, req.UserMedia...)
	}
	if req.SystemText != "" {
		req.AddSystemMessage(req.SystemText, req.SystemParams)
	}
	p, _ := request.NewChatRequestBuilder[O]().
		WithMessages(req.Messages...).
		WithOptions(req.ChatRequestOptions).
		Build()
	return p
}

func (i *invoker[O, M]) invoke(ctx *middleware.Context[O, M]) error {
	if ctx.Request.IsCall() {
		return i.callInvoke(ctx)
	}
	return i.streamInvoke(ctx)
}

func (i *invoker[O, M]) callInvoke(ctx *middleware.Context[O, M]) error {
	p := i.buildChatRequest(ctx.Request)
	resp, err := ctx.Request.ChatModel.Call(ctx.Context(), p)
	ctx.Response = resp
	return err
}

func (i *invoker[O, M]) streamInvoke(ctx *middleware.Context[O, M]) error {
	p := i.buildChatRequest(ctx.Request)
	resp, err := ctx.Request.ChatModel.Stream(ctx.Context(), p)
	ctx.Response = resp
	return err
}

func New[O request.ChatRequestOptions, M result.ChatResultMetadata]() middleware.Middleware[O, M] {
	return new(invoker[O, M]).invoke
}
