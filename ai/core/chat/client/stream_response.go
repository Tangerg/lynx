package client

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware/outputguide"
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type StreamResponse[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	Content(ctx context.Context) (string, error)
	ChatResponse(ctx context.Context) (*completion.ChatCompletion[M], error)
}

var _ StreamResponse[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultStreamResponse[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultStreamResponse[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	request *DefaultChatClientRequest[O, M]
}

func NewDefaultStreamResponseSpec[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](req *DefaultChatClientRequest[O, M]) *DefaultStreamResponse[O, M] {
	return &DefaultStreamResponse[O, M]{
		request: req,
	}
}

func (d *DefaultStreamResponse[O, M]) doGetChatResponse(ctx context.Context, format string) (*completion.ChatCompletion[M], error) {
	c := middleware.NewContext[O, M](ctx)

	if format != "" {
		c.Set(outputguide.FormatKey, format)
	}
	c.SetMap(d.request.middlewareParams)
	c.Request = d.request.toMiddlewareRequest()
	c.Request.Mode = model.StreamRequest
	c.SetMiddlewares(d.request.middlewares...)

	err := c.Next()
	if err != nil {
		return nil, err
	}

	return c.Response, nil
}

func (d *DefaultStreamResponse[O, M]) Content(ctx context.Context) (string, error) {
	resp, err := d.ChatResponse(ctx)
	if err != nil {
		return "", err
	}
	return resp.Result().Output().Content(), nil
}

func (d *DefaultStreamResponse[O, M]) ChatResponse(ctx context.Context) (*completion.ChatCompletion[M], error) {
	return d.doGetChatResponse(ctx, "")
}
