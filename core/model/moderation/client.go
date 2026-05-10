package moderation

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/core/model"
)

// Type aliases threading moderation's *Request / *Response into the
// generic [model] handler/middleware machinery.
type (
	Handler           = model.CallHandler[*Request, *Response]
	HandlerFunc       = model.CallHandlerFunc[*Request, *Response]
	Middleware        = model.CallMiddleware[*Request, *Response]
	MiddlewareManager = model.MiddlewareManager[*Request, *Response]
)

// NewMiddlewareManager returns an empty [MiddlewareManager] keyed to
// moderation's *Request / *Response pair. The stream side is unused
// (moderation has no stream endpoint).
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewMiddlewareManager[*Request, *Response]()
}

// ClientRequest is the fluent builder that turns a [Model] plus inputs
// and options into a moderation call. Construct one with
// [NewClientRequest] (or [Client.Moderate] which clones the client's
// default), chain WithXxx, then finish with [ClientRequest.Call].
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	texts             []string
	params            map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("moderation.NewClientRequest: model must not be nil")
	}
	return &ClientRequest{model: model}, nil
}

// WithMiddlewares replaces the entire middleware chain.
func (r *ClientRequest) WithMiddlewares(middlewares ...Middleware) *ClientRequest {
	if len(middlewares) > 0 {
		r.middlewareManager = NewMiddlewareManager().UseCallMiddlewares(middlewares...)
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

// WithTexts replaces the input list. Empty input is ignored.
func (r *ClientRequest) WithTexts(texts []string) *ClientRequest {
	if len(texts) > 0 {
		r.texts = texts
	}
	return r
}

// WithParams replaces the side-channel params map. Empty input is ignored.
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = params
	}
	return r
}

// MiddlewareManager returns the active manager, lazily allocating one
// if none has been set yet.
func (r *ClientRequest) MiddlewareManager() *MiddlewareManager {
	if r.middlewareManager == nil {
		r.middlewareManager = NewMiddlewareManager()
	}
	return r.middlewareManager
}

// Clone returns a deep copy.
func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:             r.model,
		middlewareManager: r.middlewareManager.Clone(),
		options:           r.options.Clone(),
		texts:             slices.Clone(r.texts),
		params:            maps.Clone(r.params),
	}
}

// resolveOptions returns the effective [Options] for this call —
// request-level options when supplied, otherwise a clone of the model's
// defaults.
func (r *ClientRequest) resolveOptions() *Options {
	if r.options != nil {
		return r.options.Clone()
	}
	return r.model.DefaultOptions().Clone()
}

// buildRequest assembles the [*Request] sent through the middleware
// chain to the underlying model.
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
//	mod, _, err := client.Moderate().WithTexts([]string{"hi"}).Call().Moderation(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}

// ClientCaller drives the synchronous moderation path.
type ClientCaller struct {
	request *ClientRequest
}

// Response runs the call through the middleware chain and returns the
// raw [*Response].
func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}
	return c.request.
		MiddlewareManager().
		BuildCallHandler(c.request.model).
		Call(ctx, req)
}

// Moderation runs the call and returns the first verdict alongside the
// full response.
func (c *ClientCaller) Moderation(ctx context.Context) (*Moderation, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resp.Result().Moderation, resp, nil
}

// Moderations runs the call and returns every verdict in input order.
func (c *ClientCaller) Moderations(ctx context.Context) ([]*Moderation, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]*Moderation, 0, len(resp.Results))
	for _, result := range resp.Results {
		out = append(out, result.Moderation)
	}
	return out, resp, nil
}

// Client wraps a [Model] with a sticky default [ClientRequest], so each
// [Client.Moderate] call clones a pre-configured starting point.
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
		return nil, errors.New("moderation.NewClientFromRequest: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

// Moderate returns a fresh clone of the default request.
func (c *Client) Moderate() *ClientRequest {
	return c.defaultRequest.Clone()
}

// ModerateWithRequest seeds a clone with the texts, options, and params
// from req.
func (c *Client) ModerateWithRequest(req *Request) *ClientRequest {
	return c.Moderate().
		WithTexts(req.Texts).
		WithOptions(req.Options).
		WithParams(req.Params)
}

// ModerateWithText is the shorthand for a single-input request.
func (c *Client) ModerateWithText(text string) *ClientRequest {
	return c.Moderate().WithTexts([]string{text})
}

// ModerateWithTexts is the shorthand for a batch request.
func (c *Client) ModerateWithTexts(texts []string) *ClientRequest {
	return c.Moderate().WithTexts(texts)
}
