package moderation

import (
	"context"
	"errors"
	"maps"
	"slices"

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

// ClientRequest is the fluent builder that turns a [Model] plus inputs
// and options into a moderation call. Construct one with
// [NewClientRequest] (or [Client.Moderate] which clones the client's
// default), chain WithXxx, then finish with [ClientRequest.Call].
type ClientRequest struct {
	model       Model
	middlewares MiddlewareChain
	options     *Options
	texts       []string
	params      map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("moderation.NewClientRequest: model must not be nil")
	}
	return &ClientRequest{model: model}, nil
}

func (r *ClientRequest) WithMiddlewares(middlewares ...Middleware) *ClientRequest {
	if len(middlewares) > 0 {
		r.middlewares = NewMiddlewareChain().WithCall(middlewares...)
	}
	return r
}

// WithOptions sets the per-request [Options]. nil is ignored.
func (r *ClientRequest) WithOptions(options *Options) *ClientRequest {
	if options != nil {
		r.options = options
	}
	return r
}

// WithTexts replaces the input list. Empty input is ignored. The
// slice is cloned so caller mutations don't leak into the request.
func (r *ClientRequest) WithTexts(texts []string) *ClientRequest {
	if len(texts) > 0 {
		r.texts = slices.Clone(texts)
	}
	return r
}

// WithParams replaces the side-channel params map. Empty input is
// ignored. The map is cloned so caller mutations don't leak.
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = maps.Clone(params)
	}
	return r
}

func (r *ClientRequest) MiddlewareChain() MiddlewareChain {
	return r.middlewares.Clone()
}

func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:       r.model,
		middlewares: r.middlewares.Clone(),
		options:     r.options.Clone(),
		texts:       slices.Clone(r.texts),
		params:      maps.Clone(r.params),
	}
}

func (r *ClientRequest) resolveOptions() *Options {
	defaults := r.model.DefaultOptions()
	if r.options != nil {
		merged, err := MergeOptions(&defaults, r.options)
		if err == nil {
			return merged
		}
	}
	return defaults.Clone()
}

func (r *ClientRequest) buildRequest() (*Request, error) {
	req, err := NewRequest(r.texts)
	if err != nil {
		return nil, err
	}
	req.Options = r.resolveOptions()
	req.Params = maps.Clone(r.params)
	return req, nil
}

// Call returns a [ClientCaller] for executing the request.
//
// Example:
//
//	cats, _, err := client.Moderate().WithTexts([]string{"hi"}).Call().Categories(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}

type ClientCaller struct {
	request *ClientRequest
}

func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}
	return c.request.
		MiddlewareChain().
		BuildCallHandler(c.request.model).
		Call(ctx, req)
}

func (c *ClientCaller) Categories(ctx context.Context) (*Categories, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resp.Result().Categories, resp, nil
}

func (c *ClientCaller) AllCategories(ctx context.Context) ([]*Categories, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]*Categories, 0, len(resp.Results))
	for _, result := range resp.Results {
		out = append(out, result.Categories)
	}
	return out, resp, nil
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
