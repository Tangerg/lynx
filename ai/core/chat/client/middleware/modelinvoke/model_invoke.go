package modelinvoke

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type invoker[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct{}

func (i *invoker[O, M]) buildPrompt(req *middleware.Request[O, M]) *prompt.ChatPrompt[O] {
	if req.UserText != "" {
		req.AddUserMessage(req.UserText)
	}
	if req.SystemText != "" {
		req.AddSystemMessage(req.SystemText)
	}
	p, _ := prompt.
		NewChatPromptBuilder[O]().
		WithMessages(req.Messages...).
		WithOptions(req.ChatOptions).
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
	p := i.buildPrompt(ctx.Request)
	resp, err := ctx.Request.ChatModel.Call(ctx.Context(), p)
	ctx.Response = resp
	return err
}

func (i *invoker[O, M]) streamInvoke(ctx *middleware.Context[O, M]) error {
	p := i.buildPrompt(ctx.Request)
	resp, err := ctx.Request.ChatModel.Stream(ctx.Context(), p)
	ctx.Response = resp
	return err
}

func New[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() middleware.Middleware[O, M] {
	i := new(invoker[O, M])
	return func(ctx *middleware.Context[O, M]) error {
		return i.invoke(ctx)
	}
}
