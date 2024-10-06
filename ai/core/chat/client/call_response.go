package client

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware/outputguide"
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/core/converter"
)

type CallResponse[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	ResponseValue(ctx context.Context, def any) (ResponseValue[any, M], error)
	ResponseValueSlice(ctx context.Context) (ResponseValue[[]string, M], error)
	ResponseValueMap(ctx context.Context, example map[string]any) (ResponseValue[map[string]any, M], error)
	ResponseValueStruct(ctx context.Context, def any) (ResponseValue[any, M], error)
	ResponseValueWithStructuredConvert(ctx context.Context, def any, c converter.StructuredConverter[any]) (ResponseValue[any, M], error)
	Content(ctx context.Context) (string, error)
	ChatResponse(ctx context.Context) (*completion.ChatCompletion[M], error)
}

var _ CallResponse[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultCallResponse[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultCallResponse[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	request *DefaultChatClientRequest[O, M]
}

func NewDefaultCallResponse[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](req *DefaultChatClientRequest[O, M]) *DefaultCallResponse[O, M] {
	return &DefaultCallResponse[O, M]{
		request: req,
	}
}

func (d *DefaultCallResponse[O, M]) doGetChatResponse(ctx context.Context, format string) (*completion.ChatCompletion[M], error) {
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

func (d *DefaultCallResponse[O, M]) ChatResponse(ctx context.Context) (*completion.ChatCompletion[M], error) {
	return d.doGetChatResponse(ctx, "")
}
