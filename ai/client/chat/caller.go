package chat

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/converter"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

type Caller struct {
	session           *Session
	middlewareManager *MiddlewareManager
}

func NewCaller(session *Session) (*Caller, error) {
	if session == nil {
		return nil, errors.New("session is required")
	}

	middlewareManager := session.middlewareManager
	if middlewareManager == nil {
		middlewareManager = NewMiddlewareManager()
	}

	return &Caller{
		session:           session,
		middlewareManager: middlewareManager.Clone(),
	}, nil
}

func (c *Caller) Execute(request *Request) (*Response, error) {
	invoker, err := newModelInvoker(request.chatModel)
	if err != nil {
		return nil, err
	}

	callHandler := c.middlewareManager.makeCallHandler(invoker)
	return callHandler.Call(request)
}

func (c *Caller) response(ctx context.Context, structuredConverter converter.StructuredConverter[any]) (*Response, error) {
	request, err := NewRequest(ctx, c.session)
	if err != nil {
		return nil, err
	}

	if structuredConverter != nil {
		request.Set(AttrChatOutputFormat.String(), structuredConverter.GetFormat())
	}

	return c.Execute(request)
}

func (c *Caller) Response(ctx context.Context) (*Response, error) {
	return c.response(ctx, nil)
}

func (c *Caller) ChatResponse(ctx context.Context) (*chat.Response, error) {
	response, err := c.response(ctx, nil)
	if err != nil {
		return nil, err
	}

	return response.ChatResponse(), nil
}

func (c *Caller) TextStructuredResponse(ctx context.Context) (*StructuredResponse[string], error) {
	response, err := c.response(ctx, nil)
	if err != nil {
		return nil, err
	}

	text := response.ChatResponse().Result().Output().Text()
	return newStructuredResponse[string](text, response), nil
}

func (c *Caller) ListStructuredResponse(ctx context.Context, listConverter ...converter.StructuredConverter[[]string]) (*StructuredResponse[[]string], error) {
	listConv := pkgSlices.FirstOr(listConverter, nil)
	if listConv == nil {
		listConv = converter.NewListConverter()
	}

	response, err := c.response(ctx, converter.AsAny(listConv))
	if err != nil {
		return nil, err
	}

	listData, err := listConv.Convert(response.ChatResponse().Result().Output().Text())
	if err != nil {
		return nil, err
	}

	return newStructuredResponse[[]string](listData, response), nil
}

func (c *Caller) MapStructuredResponse(ctx context.Context, mapConverter ...converter.StructuredConverter[map[string]any]) (*StructuredResponse[map[string]any], error) {
	mapConv := pkgSlices.FirstOr(mapConverter, nil)
	if mapConv == nil {
		mapConv = converter.NewMapConverter()
	}

	response, err := c.response(ctx, converter.AsAny(mapConv))
	if err != nil {
		return nil, err
	}

	mapData, err := mapConv.Convert(response.ChatResponse().Result().Output().Text())
	if err != nil {
		return nil, err
	}

	return newStructuredResponse[map[string]any](mapData, response), nil
}

func (c *Caller) AnyStructuredResponse(ctx context.Context, anyConverter converter.StructuredConverter[any]) (*StructuredResponse[any], error) {
	response, err := c.response(ctx, anyConverter)
	if err != nil {
		return nil, err
	}

	structuredData, err := anyConverter.Convert(response.ChatResponse().Result().Output().Text())
	if err != nil {
		return nil, err
	}

	return newStructuredResponse[any](structuredData, response), nil
}

func (c *Caller) Text(ctx context.Context) (string, error) {
	chatResponse, err := c.ChatResponse(ctx)
	if err != nil {
		return "", err
	}

	return chatResponse.Result().Output().Text(), nil
}

func (c *Caller) List(ctx context.Context, listConverter ...converter.StructuredConverter[[]string]) ([]string, error) {
	structuredResponse, err := c.ListStructuredResponse(ctx, listConverter...)
	if err != nil {
		return nil, err
	}

	return structuredResponse.Data(), nil
}

func (c *Caller) Map(ctx context.Context, mapConverter ...converter.StructuredConverter[map[string]any]) (map[string]any, error) {
	structuredResponse, err := c.MapStructuredResponse(ctx, mapConverter...)
	if err != nil {
		return nil, err
	}

	return structuredResponse.Data(), nil
}

func (c *Caller) Any(ctx context.Context, anyConverter converter.StructuredConverter[any]) (any, error) {
	structuredResponse, err := c.AnyStructuredResponse(ctx, anyConverter)
	if err != nil {
		return nil, err
	}

	return structuredResponse.Data(), nil
}
