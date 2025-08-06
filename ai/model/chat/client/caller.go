package client

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/converter"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

type Caller struct {
	config            *Config
	middlewareManager *MiddlewareManager
}

func NewCaller(config *Config) (*Caller, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}

	return &Caller{
		config:            config,
		middlewareManager: config.getMiddlewareManager(),
	}, nil
}

func (c *Caller) execute(ctx context.Context, chatRequest *chat.Request) (*chat.Response, error) {
	callHandler := c.middlewareManager.makeCallHandler(newInvoker(c.config.chatModel))
	return callHandler.Call(ctx, chatRequest)
}

func (c *Caller) Execute(ctx context.Context, chatRequest *chat.Request) (*chat.Response, error) {
	if chatRequest == nil {
		return nil, errors.New("chatRequest is required")
	}

	return c.execute(ctx, chatRequest)
}

func (c *Caller) chatResponse(ctx context.Context, structuredConverter converter.StructuredConverter[any]) (*chat.Response, error) {
	chatRequest, err := c.config.toChatRequest()
	if err != nil {
		return nil, err
	}

	if structuredConverter != nil {
		chatRequest.Set(AttrOutputFormat.String(), structuredConverter.GetFormat())
	}

	return c.execute(ctx, chatRequest)
}

func (c *Caller) ChatResponse(ctx context.Context) (*chat.Response, error) {
	return c.chatResponse(ctx, nil)
}

func (c *Caller) TextChatResponse(ctx context.Context) (string, *chat.Response, error) {
	chatResponse, err := c.chatResponse(ctx, nil)
	if err != nil {
		return "", nil, err
	}

	responseText := chatResponse.Result().Output().Text()
	return responseText, chatResponse, nil
}

func (c *Caller) ListChatResponse(ctx context.Context, listConverter ...converter.StructuredConverter[[]string]) ([]string, *chat.Response, error) {
	listConv := pkgSlices.FirstOr(listConverter, nil)
	if listConv == nil {
		listConv = converter.NewListConverter()
	}

	chatResponse, err := c.chatResponse(ctx, converter.AsAny(listConv))
	if err != nil {
		return nil, nil, err
	}

	listData, err := listConv.Convert(chatResponse.Result().Output().Text())
	return listData, chatResponse, err
}

func (c *Caller) MapChatResponse(ctx context.Context, mapConverter ...converter.StructuredConverter[map[string]any]) (map[string]any, *chat.Response, error) {
	mapConv := pkgSlices.FirstOr(mapConverter, nil)
	if mapConv == nil {
		mapConv = converter.NewMapConverter()
	}

	chatResponse, err := c.chatResponse(ctx, converter.AsAny(mapConv))
	if err != nil {
		return nil, nil, err
	}

	mapData, err := mapConv.Convert(chatResponse.Result().Output().Text())
	return mapData, chatResponse, err
}

func (c *Caller) AnyChatResponse(ctx context.Context, anyConverter converter.StructuredConverter[any]) (any, *chat.Response, error) {
	chatResponse, err := c.chatResponse(ctx, anyConverter)
	if err != nil {
		return nil, nil, err
	}

	convertedData, err := anyConverter.Convert(chatResponse.Result().Output().Text())
	return convertedData, chatResponse, err
}

func (c *Caller) Text(ctx context.Context) (string, error) {
	text, _, err := c.TextChatResponse(ctx)
	return text, err
}

func (c *Caller) List(ctx context.Context, listConverter ...converter.StructuredConverter[[]string]) ([]string, error) {
	list, _, err := c.ListChatResponse(ctx, listConverter...)
	return list, err
}

func (c *Caller) Map(ctx context.Context, mapConverter ...converter.StructuredConverter[map[string]any]) (map[string]any, error) {
	mapData, _, err := c.MapChatResponse(ctx, mapConverter...)
	return mapData, err
}

func (c *Caller) Any(ctx context.Context, anyConverter converter.StructuredConverter[any]) (any, error) {
	anyData, _, err := c.AnyChatResponse(ctx, anyConverter)
	return anyData, err
}
