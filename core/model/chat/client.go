package chat

import (
	"errors"

	"github.com/Tangerg/lynx/core/model"
)

type (
	CallHandler       = model.CallHandler[*Request, *Response]
	StreamHandler     = model.StreamHandler[*Request, *Response]
	CallHandlerFunc   = model.CallHandlerFunc[*Request, *Response]
	StreamHandlerFunc = model.StreamHandlerFunc[*Request, *Response]
	CallMiddleware    = model.CallMiddleware[*Request, *Response]
	StreamMiddleware  = model.StreamMiddleware[*Request, *Response]
	MiddlewareChain   = model.MiddlewareChain[*Request, *Response]
)

// NewMiddlewareChain returns an empty [MiddlewareChain] keyed to
// chat's *Request / *Response pair.
func NewMiddlewareChain() MiddlewareChain {
	return model.NewMiddlewareChain[*Request, *Response]()
}

// Client wraps a [Model] with a sticky default [ClientRequest], so each
// [Client.Chat] call clones a pre-configured starting point. Construct
// one with [NewClient] for the simple case, or [NewClientFromRequest]
// when you want to install default middlewares / options on the
// underlying request.
//
// Example:
//
//	client, err := chat.NewClient(model)
//	resp, err := client.Chat().WithUserPrompt("hi").Call().Response(ctx)
type Client struct {
	defaultRequest *ClientRequest
}

// NewClient is a one-step constructor: build a default [ClientRequest]
// for model, then wrap it as a [Client]. The common path.
func NewClient(model Model) (*Client, error) {
	req, err := NewClientRequest(model)
	if err != nil {
		return nil, err
	}
	return NewClientFromRequest(req)
}

// NewClientFromRequest wraps an existing [ClientRequest] as a sticky
// default — use this when the request already carries default
// middlewares / options the [Client] should keep applying.
func NewClientFromRequest(request *ClientRequest) (*Client, error) {
	if request == nil {
		return nil, errors.New("chat.NewClientFromRequest: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

// Chat returns a fresh clone of the default request, ready for fluent
// configuration without affecting the client's defaults.
func (c *Client) Chat() *ClientRequest {
	return c.defaultRequest.Clone()
}

// ChatWithRequest seeds a clone with the messages, options, and params
// from req — useful when the caller already has an assembled [Request]
// (e.g. forwarded from another service).
func (c *Client) ChatWithRequest(req *Request) *ClientRequest {
	return c.Chat().
		WithMessages(req.Messages...).
		WithOptions(req.Options).
		WithParams(req.Params)
}

// ChatWithText is the most common shortcut: a single user-message turn.
func (c *Client) ChatWithText(text string) *ClientRequest {
	return c.Chat().WithMessages(NewUserMessage(text))
}

// ChatWithPromptTemplate seeds a clone with the given user-prompt
// template — render it later via [ClientCaller] / [ClientStreamer].
func (c *Client) ChatWithPromptTemplate(template *PromptTemplate) *ClientRequest {
	return c.Chat().WithUserPromptTemplate(template)
}
