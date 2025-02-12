package client

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/converter"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// CallResponse is a generic interface defining the contract for handling responses
// from call-based chat requests in a chat application. It allows retrieval of
// response values in various formats and the full chat response.
//
// Type Parameters:
//   - O: Represents the chat options, typically defined by request.ChatRequestOptions.
//   - M: Represents the metadata associated with chat generation, typically defined by result.ChatResultMetadata.
//
// Methods:
//
// ResponseValue:
//
//	ResponseValue(ctx context.Context, def any) (ResponseValue[any, M], error)
//	Retrieves a response value of any type, using the provided context for managing
//	request-scoped values. A default value (def) can be provided to use if the
//	response value is unavailable.
//	Returns:
//	  - ResponseValue[any, M]: A container for the result and metadata.
//	  - error: An error if any issues occur during retrieval.
//
// ResponseValueSlice:
//
//	ResponseValueSlice(ctx context.Context) (ResponseValue[[]string, M], error)
//	Retrieves a response value as a slice of strings, using the provided context.
//	Returns:
//	  - ResponseValue[[]string, M]: A container for the result and metadata.
//	  - error: An error if any issues occur during retrieval.
//
// ResponseValueMap:
//
//	ResponseValueMap(ctx context.Context, example map[string]any) (ResponseValue[map[string]any, M], error)
//	Retrieves a response value as a map, using the provided context. An example
//	map can be passed to guide the structure of the response value.
//	Returns:
//	  - ResponseValue[map[string]any, M]: A container for the result and metadata.
//	  - error: An error if any issues occur during retrieval.
//
// ResponseValueStruct:
//
//	ResponseValueStruct(ctx context.Context, def any) (ResponseValue[any, M], error)
//	Retrieves a response value as a structured type, using the provided context.
//	A default value (def) can be provided to use if the response value is unavailable.
//	Returns:
//	  - ResponseValue[any, M]: A container for the result and metadata.
//	  - error: An error if any issues occur during retrieval.
//
// ResponseValueWithStructuredConvert:
//
//	ResponseValueWithStructuredConvert(ctx context.Context, def any, c converter.StructuredConverter[any]) (ResponseValue[any, M], error)
//	Retrieves a response value with structured conversion, using the provided context.
//	A default value (def) and a converter can be provided to transform the response
//	value into a structured format.
//	Returns:
//	  - ResponseValue[any, M]: A container for the result and metadata.
//	  - error: An error if any issues occur during retrieval.
//
// Content:
//
//	Content(ctx context.Context) (string, error)
//	Retrieves the content of the response as a string, using the provided context.
//	Returns:
//	  - string: The content of the response.
//	  - error: An error if any issues occur during retrieval.
//
// ChatResponse:
//
//	ChatResponse(ctx context.Context) (*response.ChatResponse[M], error)
//	Retrieves the full chat response as a ChatResponse.
//	Returns:
//	  - *response.ChatResponse[M]: A pointer to the ChatResponse instance.
//	  - error: An error if any issues occur during retrieval.
//
// Example Usage:
//
//	var callResp CallResponse[ChatOptions, ChatMetadata]
//	content, err := callResp.Content(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Response content:", content)
type CallResponse[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	// ResponseValue Retrieves a response value of any type, using the provided context and a default value.
	ResponseValue(ctx context.Context, def any) (ResponseValue[any, M], error)

	// ResponseValueSlice Retrieves a response value as a slice of strings.
	ResponseValueSlice(ctx context.Context) (ResponseValue[[]string, M], error)

	// ResponseValueMap Retrieves a response value as a map, using an example map to guide its structure.
	ResponseValueMap(ctx context.Context, example map[string]any) (ResponseValue[map[string]any, M], error)

	// ResponseValueStruct Retrieves a response value as a structured type, using a default value if unavailable.
	ResponseValueStruct(ctx context.Context, def any) (ResponseValue[any, M], error)

	// ResponseValueWithStructuredConvert Retrieves a response value with structured conversion, using a default value and converter.
	ResponseValueWithStructuredConvert(ctx context.Context, def any, c converter.StructuredConverter[any]) (ResponseValue[any, M], error)

	// Content Retrieves the content of the response as a string.
	Content(ctx context.Context) (string, error)

	// ChatResponse Retrieves the full chat response as a ChatResponse.
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
		c.Set(middleware.ResponseFormatKey, format)
	}
	c.SetMap(d.request.middlewareParams)
	c.Request = d.request.toMiddlewareRequest(middleware.CallRequest)
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
	rv.response = resp
	convert, err := c.Convert(resp.Result().Output().Content())
	if err != nil {
		return rv, err
	}
	rv.value = convert

	return rv, nil
}

func (d *DefaultCallResponse[O, M]) ResponseValueMap(ctx context.Context, example map[string]any) (ResponseValue[map[string]any, M], error) {
	c := converter.NewMapConverterWithExample(example)
	resp, err := d.doGetChatResponse(ctx, c.GetFormat())
	if err != nil {
		return nil, err
	}

	rv := NewDefaultResponseValue[map[string]any, M](map[string]any{})
	rv.response = resp
	convert, err := c.Convert(resp.Result().Output().Content())
	if err != nil {
		return rv, err
	}
	rv.value = convert

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
	rv.response = resp
	convert, err := c.Convert(resp.Result().Output().Content())
	if err != nil {
		return rv, err
	}
	rv.value = convert

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
