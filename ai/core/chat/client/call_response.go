package client

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware/outputguide"
	"github.com/Tangerg/lynx/ai/core/chat/converter"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// CallResponse is a generic interface that defines the contract for handling responses
// from a call-based chat request in a chat application. It is parameterized by chat options (O)
// and chat generation metadata (M).
//
// Type Parameters:
//   - O: Represents the chat options, defined by the prompt.ChatOptions type.
//   - M: Represents the metadata associated with chat generation, defined by the metadata.ChatGenerationMetadata type.
//
// Methods:
//
// ResponseValue(ctx context.Context, def any) (ResponseValue[any, M], error)
//   - Retrieves a response value of any type, using the provided context for managing request-scoped values.
//   - Takes a default value (def) to be used if the response value is not available.
//   - Returns a ResponseValue containing the result and an error if any issues occur.
//
// ResponseValueSlice(ctx context.Context) (ResponseValue[[]string, M], error)
//   - Retrieves a response value as a slice of strings, using the provided context.
//   - Returns a ResponseValue containing the result and an error if any issues occur.
//
// ResponseValueMap(ctx context.Context, example map[string]any) (ResponseValue[map[string]any, M], error)
//   - Retrieves a response value as a map, using the provided context.
//   - Takes an example map to guide the structure of the response value.
//   - Returns a ResponseValue containing the result and an error if any issues occur.
//
// ResponseValueStruct(ctx context.Context, def any) (ResponseValue[any, M], error)
//   - Retrieves a response value as a structured type, using the provided context.
//   - Takes a default value (def) to be used if the response value is not available.
//   - Returns a ResponseValue containing the result and an error if any issues occur.
//
// ResponseValueWithStructuredConvert(ctx context.Context, def any, c converter.StructuredConverter[any]) (ResponseValue[any, M], error)
//   - Retrieves a response value with a structured conversion, using the provided context.
//   - Takes a default value (def) and a converter to transform the response value into a structured format.
//   - Returns a ResponseValue containing the result and an error if any issues occur.
//
// Content(ctx context.Context) (string, error)
//   - Retrieves the content of the response as a string, using the provided context.
//   - Returns the content and an error if any issues occur.
//
// ChatResponse(ctx context.Context) (*completion.ChatCompletion[M], error)
//   - Retrieves the full chat response as a ChatCompletion, using the provided context.
//   - Returns a pointer to the ChatCompletion and an error if any issues occur.
type CallResponse[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	ResponseValue(ctx context.Context, def any) (ResponseValue[any, M], error)
	ResponseValueSlice(ctx context.Context) (ResponseValue[[]string, M], error)
	ResponseValueMap(ctx context.Context, example map[string]any) (ResponseValue[map[string]any, M], error)
	ResponseValueStruct(ctx context.Context, def any) (ResponseValue[any, M], error)
	ResponseValueWithStructuredConvert(ctx context.Context, def any, c converter.StructuredConverter[any]) (ResponseValue[any, M], error)
	Content(ctx context.Context) (string, error)
	ChatResponse(ctx context.Context) (*response.ChatResponse[M], error)
}

var _ CallResponse[request.ChatRequestOptions, result.ChatResultMetadata] = (*DefaultCallResponse[request.ChatRequestOptions, result.ChatResultMetadata])(nil)

type DefaultCallResponse[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	request *DefaultChatClientRequest[O, M]
}

func NewDefaultCallResponse[O request.ChatRequestOptions, M result.ChatResultMetadata](req *DefaultChatClientRequest[O, M]) *DefaultCallResponse[O, M] {
	return &DefaultCallResponse[O, M]{
		request: req,
	}
}

func (d *DefaultCallResponse[O, M]) doGetChatResponse(ctx context.Context, format string) (*response.ChatResponse[M], error) {
	c := middleware.NewContext[O, M](ctx)

	if format != "" {
		c.Set(outputguide.FormatKey, format)
	}
	c.SetMap(d.request.middlewareParams)
	c.Request = d.request.toMiddlewareRequest()
	c.SetMiddlewares(d.request.middlewares...)

	err := c.Next()
	if err != nil {
		return nil, err
	}

	return c.Response, nil
}

func (d *DefaultCallResponse[O, M]) ResponseValue(ctx context.Context, def any) (ResponseValue[any, M], error) {
	return d.ResponseValueStruct(ctx, def)
}

func (d *DefaultCallResponse[O, M]) ResponseValueSlice(ctx context.Context) (ResponseValue[[]string, M], error) {
	c := converter.NewSliceConverter()
	resp, err := d.doGetChatResponse(ctx, c.GetFormat())
	if err != nil {
		return nil, err
	}

	rv := NewDefaultResponseValue[[]string, M]([]string{})
	convert, err := c.Convert(resp.Result().Output().Content())
	if err != nil {
		return rv, err
	}
	rv.value = convert
	rv.response = resp

	return rv, nil
}

func (d *DefaultCallResponse[O, M]) ResponseValueMap(ctx context.Context, example map[string]any) (ResponseValue[map[string]any, M], error) {
	c := converter.NewMapConverterWithExample(example)
	resp, err := d.doGetChatResponse(ctx, c.GetFormat())
	if err != nil {
		return nil, err
	}

	rv := NewDefaultResponseValue[map[string]any, M](map[string]any{})
	convert, err := c.Convert(resp.Result().Output().Content())
	if err != nil {
		return rv, err
	}
	rv.value = convert
	rv.response = resp

	return rv, nil
}

func (d *DefaultCallResponse[O, M]) ResponseValueStruct(ctx context.Context, def any) (ResponseValue[any, M], error) {
	return d.ResponseValueWithStructuredConvert(
		ctx,
		def,
		converter.NewStructConverterWithDefault(def),
	)
}

func (d *DefaultCallResponse[O, M]) ResponseValueWithStructuredConvert(ctx context.Context, def any, c converter.StructuredConverter[any]) (ResponseValue[any, M], error) {
	resp, err := d.doGetChatResponse(ctx, c.GetFormat())
	if err != nil {
		return nil, err
	}

	rv := NewDefaultResponseValue[any, M](def)

	convert, err := c.Convert(resp.Result().Output().Content())
	if err != nil {
		return rv, err
	}
	rv.value = convert
	rv.response = resp

	return rv, nil

}

func (d *DefaultCallResponse[O, M]) Content(ctx context.Context) (string, error) {
	resp, err := d.ChatResponse(ctx)
	if err != nil {
		return "", err
	}
	return resp.Result().Output().Content(), nil
}

func (d *DefaultCallResponse[O, M]) ChatResponse(ctx context.Context) (*response.ChatResponse[M], error) {
	return d.doGetChatResponse(ctx, "")
}
