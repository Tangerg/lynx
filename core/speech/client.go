package speech

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

func NewMiddlewareChain() MiddlewareChain {
	return model.NewMiddlewareChain[*Request, *Response]()
}

// Client wraps a [Model] with a sticky default [ClientRequest], so each
// [Client.Synthesize] call clones a pre-configured starting point.
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
		return nil, errors.New("tts.NewClientFromRequest: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

func (c *Client) Synthesize() *ClientRequest {
	return c.defaultRequest.Clone()
}

func (c *Client) SynthesizeWithRequest(req *Request) *ClientRequest {
	return c.Synthesize().
		WithText(req.Text).
		WithOptions(req.Options).
		WithParams(req.Params)
}

func (c *Client) SynthesizeWithText(text string) *ClientRequest {
	return c.Synthesize().WithText(text)
}
