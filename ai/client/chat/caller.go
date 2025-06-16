package chat

import (
	"context"
	"errors"
	"github.com/Tangerg/lynx/ai/model/chat"

	"github.com/Tangerg/lynx/ai/model/converter"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

type Caller struct {
	options     *Options
	middleWares *Middlewares
}

func NewCaller(options *Options) (*Caller, error) {
	if options == nil {
		return nil, errors.New("options is required")
	}

	middleWares := options.middlewares
	if middleWares == nil {
		middleWares = NewMiddlewares()
	}

	return &Caller{
		options:     options,
		middleWares: middleWares.Clone(),
	}, nil
}

func (c *Caller) response(ctx context.Context, converter converter.StructuredConverter[any]) (*Response, error) {
	request, err := NewRequest(ctx, c.options)
	if err != nil {
		return nil, err
	}
	if converter != nil {
		request.Set(OutputFormat.String(), converter.GetFormat())
	}
	return c.Execute(request)
}

func (c *Caller) Execute(request *Request) (*Response, error) {
	invoker, err := newModelInvoker(request.chatModel)
	if err != nil {
		return nil, err
	}
	callHandler := c.middleWares.makeCallHandler(invoker)
	return callHandler.Call(request)
}

func (c *Caller) Response(ctx context.Context) (*Response, error) {
	return c.response(ctx, nil)
}

func (c *Caller) TextStructuredResponse(ctx context.Context) (*StructuredResponse[string], error) {
	resp, err := c.response(ctx, nil)
	if err != nil {
		return nil, err
	}
	text := resp.ChatResponse().Result().Output().Text()
	return newStructuredResponse[string](text, resp), nil
}

func (c *Caller) ListStructuredResponse(ctx context.Context, listConverter ...converter.StructuredConverter[[]string]) (*StructuredResponse[[]string], error) {
	lc, _ := pkgSlices.At(listConverter, 0)
	if lc == nil {
		lc = converter.NewListConverter()
	}
	resp, err := c.response(ctx, converter.AsAny(lc))
	if err != nil {
		return nil, err
	}
	list, err := lc.Convert(resp.ChatResponse().Result().Output().Text())
	if err != nil {
		return nil, err
	}
	return newStructuredResponse[[]string](list, resp), nil
}

func (c *Caller) MapStructuredResponse(ctx context.Context, mapConverter ...converter.StructuredConverter[map[string]any]) (*StructuredResponse[map[string]any], error) {
	mc, _ := pkgSlices.At(mapConverter, 0)
	if mc == nil {
		mc = converter.NewMapConverter()
	}
	resp, err := c.response(ctx, converter.AsAny(mc))
	if err != nil {
		return nil, err
	}
	m, err := mc.Convert(resp.ChatResponse().Result().Output().Text())
	if err != nil {
		return nil, err
	}
	return newStructuredResponse[map[string]any](m, resp), nil
}

func (c *Caller) AnyStructuredResponse(ctx context.Context, converter converter.StructuredConverter[any]) (*StructuredResponse[any], error) {
	resp, err := c.response(ctx, converter)
	if err != nil {
		return nil, err
	}
	structured, err := converter.Convert(resp.ChatResponse().Result().Output().Text())
	if err != nil {
		return nil, err
	}
	return newStructuredResponse[any](structured, resp), nil
}

func (c *Caller) Text(ctx context.Context) (string, error) {
	resp, err := c.ChatResponse(ctx)
	if err != nil {
		return "", err
	}
	return resp.Result().Output().Text(), nil
}

func (c *Caller) List(ctx context.Context, listConverter ...converter.StructuredConverter[[]string]) ([]string, error) {
	resp, err := c.ListStructuredResponse(ctx, listConverter...)
	if err != nil {
		return nil, err
	}
	return resp.Data(), nil
}

func (c *Caller) Map(ctx context.Context, mapConverter ...converter.StructuredConverter[map[string]any]) (map[string]any, error) {
	resp, err := c.MapStructuredResponse(ctx, mapConverter...)
	if err != nil {
		return nil, err
	}
	return resp.Data(), nil
}

func (c *Caller) Any(ctx context.Context, converter converter.StructuredConverter[any]) (any, error) {
	resp, err := c.AnyStructuredResponse(ctx, converter)
	if err != nil {
		return nil, err
	}
	return resp.Data(), nil
}

func (c *Caller) ChatResponse(ctx context.Context) (*chat.Response, error) {
	resp, err := c.response(ctx, nil)
	if err != nil {
		return nil, err
	}
	return resp.ChatResponse(), nil
}
