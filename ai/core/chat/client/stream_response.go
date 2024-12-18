package client

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
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
type StreamResponse[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	Content(ctx context.Context) (string, error)
	ChatResponse(ctx context.Context) (*response.ChatResponse[M], error)
}

var _ StreamResponse[request.ChatRequestOptions, result.ChatResultMetadata] = (*DefaultStreamResponse[request.ChatRequestOptions, result.ChatResultMetadata])(nil)

type DefaultStreamResponse[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	request *DefaultChatClientRequest[O, M]
}

func NewDefaultStreamResponseSpec[O request.ChatRequestOptions, M result.ChatResultMetadata](req *DefaultChatClientRequest[O, M]) *DefaultStreamResponse[O, M] {
	return &DefaultStreamResponse[O, M]{
		request: req,
	}
}

func (d *DefaultStreamResponse[O, M]) doGetChatResponse(ctx context.Context, format string) (*response.ChatResponse[M], error) {
	c := middleware.NewContext[O, M](ctx)

	if format != "" {
		c.Set(middleware.ResponseFormatKey, format)
	}
	c.SetMap(d.request.middlewareParams)
	c.Request = d.request.toMiddlewareRequest(middleware.StreamRequest)
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

func (d *DefaultStreamResponse[O, M]) ChatResponse(ctx context.Context) (*response.ChatResponse[M], error) {
	return d.doGetChatResponse(ctx, "")
}
