package image

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
// image's *Request / *Response pair. The stream side is unused (image
// generation has no stream endpoint).
func NewMiddlewareChain() MiddlewareChain {
	return model.NewMiddlewareChain[*Request, *Response]()
}

// Client wraps a [Model] with a sticky default [ClientRequest], so each
// [Client.Generate] call clones a pre-configured starting point.
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
		return nil, errors.New("image.NewClientFromRequest: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

func (c *Client) Generate() *ClientRequest {
	return c.defaultRequest.Clone()
}

func (c *Client) GenerateWithRequest(req *Request) *ClientRequest {
	return c.Generate().
		WithPrompt(req.Prompt).
		WithOptions(req.Options).
		WithParams(req.Params)
}

func (c *Client) GenerateWithPrompt(prompt string) *ClientRequest {
	return c.Generate().WithPrompt(prompt)
}
