package image

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/lynx/core/model"
)

type (
	Handler           = model.CallHandler[*Request, *Response]
	HandlerFunc       = model.CallHandlerFunc[*Request, *Response]
	Middleware        = model.CallMiddleware[*Request, *Response]
	MiddlewareManager = model.MiddlewareManager[*Request, *Response]
)

// The stream side is unused (image generation has no stream endpoint).
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

// Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("image.NewClientRequest: model must not be nil")
	}
	return &ClientRequest{model: model}, nil
}

func (r *ClientRequest) WithMiddlewares(middlewares ...Middleware) *ClientRequest {
	if len(middlewares) > 0 {
		r.middlewareManager = NewMiddlewareManager().UseCallMiddlewares(middlewares...)
	}
	return r
}

// nil is ignored.
func (r *ClientRequest) WithOptions(options *Options) *ClientRequest {
	if options != nil {
		r.options = options
	}
	return r
}

// Empty input is ignored.
func (r *ClientRequest) WithPrompt(prompt string) *ClientRequest {
	if prompt != "" {
		r.prompt = prompt
	}
	return r
}

// Empty input is
// ignored. The map is cloned so caller mutations don't leak.
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = maps.Clone(params)
	}
	return r
}

func (r *ClientRequest) MiddlewareManager() *MiddlewareManager {
	if r.middlewareManager == nil {
		r.middlewareManager = NewMiddlewareManager()
	}
	return r.middlewareManager
}

func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:             r.model,
		middlewareManager: r.middlewareManager.Clone(),
		options:           r.options.Clone(),
		prompt:            r.prompt,
		params:            maps.Clone(r.params),
	}
}

func (r *ClientRequest) resolveOptions() *Options {
	if r.options != nil {
		return r.options.Clone()
	}
	defaults := r.model.DefaultOptions()
	return defaults.Clone()
}

func (r *ClientRequest) buildRequest() (*Request, error) {
	req, err := NewRequest(r.prompt)
	if err != nil {
		return nil, err
	}
	req.Options = r.resolveOptions()
	req.Params = maps.Clone(r.params)
	return req, nil
}

// Example:
//
//	img, _, err := client.Generate().WithPrompt("a duck").Call().Image(ctx)
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
		MiddlewareManager().
		BuildCallHandler(c.request.model).
		Call(ctx, req)
}

func (c *ClientCaller) Image(ctx context.Context) (*Image, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resp.Result.Image, resp, nil
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

// Use this when the request already carries default
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
