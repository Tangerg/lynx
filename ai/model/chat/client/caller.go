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

func (c *Caller) execute(ctx context.Context, chatModel chat.Model, chatRequest *chat.Request) (*chat.Response, error) {
	callHandler := c.middlewareManager.makeCallHandler(newInvoker(chatModel))
	return callHandler.Call(ctx, chatRequest)
}

func (c *Caller) Execute(ctx context.Context, chatModel chat.Model, chatRequest *chat.Request) (*chat.Response, error) {
	if chatModel == nil {
		return nil, errors.New("chatModel is required")
	}
	if chatRequest == nil {
		return nil, errors.New("chatRequest is required")
	}

	return c.execute(ctx, chatModel, chatRequest)
}

func (c *Caller) chatResponse(ctx context.Context, structuredConverter converter.StructuredConverter[any]) (*chat.Response, error) {
	chatRequest, err := c.config.toChatRequest()
	if err != nil {
		return nil, err
	}

	if structuredConverter != nil {
		chatRequest.Set(AttrOutputFormat.String(), structuredConverter.GetFormat())
	}

	return c.execute(ctx, c.config.chatModel, chatRequest)
}

func (c *Caller) ChatResponse(ctx context.Context) (*chat.Response, error) {
	return c.chatResponse(ctx, nil)
}

func (c *Caller) TextChatResponse(ctx context.Context) (*StructuredResponse[string], error) {
	chatResponse, err := c.chatResponse(ctx, nil)
	if err != nil {
		return nil, err
	}

	responseText := chatResponse.Result().Output().Text()
	return newStructuredResponse(responseText, chatResponse), nil
}

func (c *Caller) ListChatResponse(ctx context.Context, listConverter ...converter.StructuredConverter[[]string]) (*StructuredResponse[[]string], error) {
	listConv := pkgSlices.FirstOr(listConverter, nil)
	if listConv == nil {
		listConv = converter.NewListConverter()
	}

	chatResponse, err := c.chatResponse(ctx, converter.AsAny(listConv))
	if err != nil {
		return nil, err
	}

	listData, err := listConv.Convert(chatResponse.Result().Output().Text())
	if err != nil {
		return nil, err
	}

	return newStructuredResponse(listData, chatResponse), nil
}

func (c *Caller) MapChatResponse(ctx context.Context, mapConverter ...converter.StructuredConverter[map[string]any]) (*StructuredResponse[map[string]any], error) {
	mapConv := pkgSlices.FirstOr(mapConverter, nil)
	if mapConv == nil {
		mapConv = converter.NewMapConverter()
	}

	chatResponse, err := c.chatResponse(ctx, converter.AsAny(mapConv))
	if err != nil {
		return nil, err
	}

	mapData, err := mapConv.Convert(chatResponse.Result().Output().Text())
	if err != nil {
		return nil, err
	}

	return newStructuredResponse(mapData, chatResponse), nil
}

func (c *Caller) AnyChatResponse(ctx context.Context, anyConverter converter.StructuredConverter[any]) (*StructuredResponse[any], error) {
	chatResponse, err := c.chatResponse(ctx, anyConverter)
	if err != nil {
		return nil, err
	}

	convertedData, err := anyConverter.Convert(chatResponse.Result().Output().Text())
	if err != nil {
		return nil, err
	}

	return newStructuredResponse(convertedData, chatResponse), nil
}

func (c *Caller) Text(ctx context.Context) (string, error) {
	structuredResponse, err := c.TextChatResponse(ctx)
	if err != nil {
		return "", err
	}

	return structuredResponse.Value(), nil
}

func (c *Caller) List(ctx context.Context, listConverter ...converter.StructuredConverter[[]string]) ([]string, error) {
	structuredResponse, err := c.ListChatResponse(ctx, listConverter...)
	if err != nil {
		return nil, err
	}

	return structuredResponse.Value(), nil
}

func (c *Caller) Map(ctx context.Context, mapConverter ...converter.StructuredConverter[map[string]any]) (map[string]any, error) {
	structuredResponse, err := c.MapChatResponse(ctx, mapConverter...)
	if err != nil {
		return nil, err
	}

	return structuredResponse.Value(), nil
}

func (c *Caller) Any(ctx context.Context, anyConverter converter.StructuredConverter[any]) (any, error) {
	structuredResponse, err := c.AnyChatResponse(ctx, anyConverter)
	if err != nil {
		return nil, err
	}

	return structuredResponse.Value(), nil
}
