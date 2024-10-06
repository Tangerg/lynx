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

// StreamResponse is a generic interface that defines the contract for handling responses
// from a stream-based chat request in a chat application. It is parameterized by chat options (O)
// and chat generation metadata (M).
//
// Type Parameters:
//   - O: Represents the chat options, defined by the prompt.ChatOptions type.
//   - M: Represents the metadata associated with chat generation, defined by the metadata.ChatGenerationMetadata type.
//
// Methods:
//
// Content(ctx context.Context) (string, error)
//   - Retrieves the content of the response as a string, using the provided context for managing request-scoped values.
//   - Returns the content and an error if any issues occur during the retrieval process.
//   - This method is useful for obtaining a textual representation of the streamed response.
//
// ChatResponse(ctx context.Context) (*completion.ChatCompletion[M], error)
//   - Retrieves the full chat response as a ChatCompletion, using the provided context.
//   - Returns a pointer to the ChatCompletion and an error if any issues occur during the retrieval process.
//   - This method provides access to the complete response object, including metadata and other relevant information.
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
