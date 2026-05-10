package image

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/lynx/core/model"
)

// Type aliases threading image's *Request / *Response into the generic
// [model] handler/middleware machinery.
type (
	Handler           = model.CallHandler[*Request, *Response]
	HandlerFunc       = model.CallHandlerFunc[*Request, *Response]
	Middleware        = model.CallMiddleware[*Request, *Response]
	MiddlewareManager = model.MiddlewareManager[*Request, *Response]
)

// NewMiddlewareManager returns an empty [MiddlewareManager] keyed to
// image's *Request / *Response pair. The stream side is unused (image
// generation has no stream endpoint).
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewMiddlewareManager[*Request, *Response]()
}

// ClientRequest is the fluent builder that turns a [Model] plus a
// prompt and options into an image-generation call. Construct one with
// [NewClientRequest] (or [Client.Generate] which clones the client's
// default), chain WithXxx, then finish with [ClientRequest.Call].
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	prompt            string
	params            map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("image.NewClientRequest: model must not be nil")
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

// WithPrompt sets the prompt text. Empty input is ignored.
func (r *ClientRequest) WithPrompt(prompt string) *ClientRequest {
	if prompt != "" {
		r.prompt = prompt
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
		prompt:            r.prompt,
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
	req, err := NewRequest(r.prompt)
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
//	img, _, err := client.Generate().WithPrompt("a duck").Call().Image(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}

// ClientCaller drives the synchronous image-generation path.
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

// Image runs the call and returns the first generated image alongside
// the full response.
func (c *ClientCaller) Image(ctx context.Context) (*Image, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resp.Result().Image, resp, nil
}

// Images runs the call and returns every generated image in order.
func (c *ClientCaller) Images(ctx context.Context) ([]*Image, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]*Image, 0, len(resp.Results))
	for _, result := range resp.Results {
		out = append(out, result.Image)
	}
	return out, resp, nil
}

// Client wraps a [Model] with a sticky default [ClientRequest], so each
// [Client.Generate] call clones a pre-configured starting point.
type Client struct {
	defaultRequest *ClientRequest
}

// NewClient wraps an existing [ClientRequest] as a sticky default.
// Returns an error when request is nil.
func NewClient(request *ClientRequest) (*Client, error) {
	if request == nil {
		return nil, errors.New("image.NewClient: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

// NewClientWithModel is a one-step constructor.
func NewClientWithModel(model Model) (*Client, error) {
	req, err := NewClientRequest(model)
	if err != nil {
		return nil, err
	}
	return NewClient(req)
}

// Generate returns a fresh clone of the default request.
func (c *Client) Generate() *ClientRequest {
	return c.defaultRequest.Clone()
}

// GenerateWithRequest seeds a clone with the prompt, options, and
// params from req.
func (c *Client) GenerateWithRequest(req *Request) *ClientRequest {
	return c.Generate().
		WithPrompt(req.Prompt).
		WithOptions(req.Options).
		WithParams(req.Params)
}

// GenerateWithPrompt is the most common shortcut.
func (c *Client) GenerateWithPrompt(prompt string) *ClientRequest {
	return c.Generate().WithPrompt(prompt)
}
