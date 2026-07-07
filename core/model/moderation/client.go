package moderation

import (
	"errors"

	"github.com/Tangerg/lynx/core/model"
)

type (
	Handler         = model.CallHandler[*Request, *Response]
	HandlerFunc     = model.CallHandlerFunc[*Request, *Response]
	Middleware      = model.CallMiddleware[*Request, *Response]
	MiddlewareChain = model.MiddlewareChain[*Request, *Response]
)

// NewMiddlewareChain returns an empty [MiddlewareChain] keyed to
// moderation's *Request / *Response pair. The stream side is unused
// (moderation has no stream endpoint).
func NewMiddlewareChain() MiddlewareChain {
	return model.NewMiddlewareChain[*Request, *Response]()
}

// Client wraps a [Model] with a sticky default [ClientRequest], so each
// [Client.Moderate] call clones a pre-configured starting point.
type Client struct {
	defaultRequest *ClientRequest
}

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
		return nil, errors.New("moderation.NewClientFromRequest: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

func (c *Client) Moderate() *ClientRequest {
	return c.defaultRequest.Clone()
}

func (c *Client) ModerateWithRequest(req *Request) *ClientRequest {
	return c.Moderate().
		WithTexts(req.Texts).
		WithOptions(req.Options).
		WithParams(req.Params)
}

func (c *Client) ModerateWithText(text string) *ClientRequest {
	return c.Moderate().WithTexts([]string{text})
}

func (c *Client) ModerateWithTexts(texts []string) *ClientRequest {
	return c.Moderate().WithTexts(texts)
}
