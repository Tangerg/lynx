package chat

import (
	"context"
	"errors"
	"time"
)

// ClientCaller drives the synchronous chat path. Build it via
// [ClientRequest.Call]; finish via [ClientCaller.Response],
// [ClientCaller.Text], or [ClientCaller.Structured].
type ClientCaller struct {
	request *ClientRequest
}

// call feeds the request through the middleware chain into the model.
// Tool execution is NOT auto-injected; register the call/stream middleware
// pair for your loop driver via WithCallMiddlewares and WithStreamMiddlewares
// if you need that.
//
// One OTel span is started per call, following the GenAI semconv. When
// no TracerProvider is configured, the span calls are no-op.
func (c *ClientCaller) call(ctx context.Context, req *Request) (*Response, error) {
	start := time.Now()
	ctx, span := startChatSpan(ctx, c.request.model, req, "chat")
	handler := c.request.MiddlewareChain().BuildCallHandler(c.request.model)
	resp, err := handler.Call(ctx, req)
	finishChatSpan(span, resp, err)
	recordChatMetrics(ctx, c.request.model, req, resp, err, start)
	return resp, err
}

func (c *ClientCaller) runCall(ctx context.Context, parser StructuredParser[any]) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}

	if parser != nil {
		req.AppendToLastUserMessage(parser.Instructions())
	}

	return c.call(ctx, req)
}

func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	return c.runCall(ctx, nil)
}

func (c *ClientCaller) Text(ctx context.Context) (string, *Response, error) {
	resp, err := c.runCall(ctx, nil)
	if err != nil {
		return "", nil, err
	}
	if resp.Result == nil || resp.Result.AssistantMessage == nil {
		return "", resp, errors.New("chat.ClientCaller.Text: response carries no assistant message")
	}
	return resp.Result.AssistantMessage.JoinedText(), resp, nil
}

// Structured runs the call with parser-supplied prompt instructions
// then decodes the assistant's text into the parser's typed value.
//
// Example:
//
//	parser := chat.NewJSONParser[Recipe]()
//	any, _, err := client.Chat().WithUserPrompt("...").Call().Structured(ctx, chat.WrapParserAsAny(parser))
func (c *ClientCaller) Structured(ctx context.Context, parser StructuredParser[any]) (any, *Response, error) {
	resp, err := c.runCall(ctx, parser)
	if err != nil {
		return nil, nil, err
	}
	if resp.Result == nil || resp.Result.AssistantMessage == nil {
		return nil, resp, errors.New("chat.ClientCaller.Structured: response carries no assistant message")
	}
	data, parseErr := parser.Parse(resp.Result.AssistantMessage.JoinedText())
	return data, resp, parseErr
}
