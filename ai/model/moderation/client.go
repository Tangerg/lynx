package moderation

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/lynx/ai/model"
)

type Handler = model.CallHandler[*Request, *Response]
type HandlerFunc = model.CallHandlerFunc[*Request, *Response]
type Middleware = model.CallMiddleware[*Request, *Response]
type MiddlewareManager = model.CallMiddlewareManager[*Request, *Response]

// NewMiddlewareManager creates a new middleware manager for moderation requests
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewCallMiddlewareManager[*Request, *Response]()
}

// ClientRequest represents a fluent builder for constructing moderation requests
// It allows configuring the model, middlewares, options, texts, and additional parameters
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	texts             []string
	params            map[string]any
}

// NewClientRequest creates a new ClientRequest with the specified moderation model
// Returns an error if the model is nil
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("model is nil")
	}
	return &ClientRequest{
		model: model,
	}, nil
}

// WithMiddlewares adds middlewares to the request
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithMiddlewares(middlewares ...Middleware) *ClientRequest {
	if len(middlewares) > 0 {
		r.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewares...)
	}
	return r
}

// WithMiddlewareManager sets a custom middleware manager for the request
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithMiddlewareManager(middlewareManager *MiddlewareManager) *ClientRequest {
	if middlewareManager != nil {
		r.middlewareManager = middlewareManager
	}
	return r
}

// WithOptions sets the configuration options for the request
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithOptions(options *Options) *ClientRequest {
	if options != nil {
		r.options = options
	}
	return r
}

// WithTexts sets the text contents to be moderated
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithTexts(texts []string) *ClientRequest {
	if len(texts) > 0 {
		r.texts = texts
	}
	return r
}

// WithParams sets additional parameters for the request
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = params
	}
	return r
}

// MiddlewareManager returns the middleware manager for this request
// Creates a new one if it doesn't exist
func (r *ClientRequest) MiddlewareManager() *MiddlewareManager {
	if r.middlewareManager == nil {
		r.middlewareManager = NewMiddlewareManager()
	}
	return r.middlewareManager
}

// Clone creates a deep copy of the ClientRequest
func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:             r.model,
		middlewareManager: r.middlewareManager.Clone(),
		options:           r.options.Clone(),
		texts:             r.texts,
		params:            maps.Clone(r.params),
	}
}

// getOptions returns the options to use for the request
// Uses the explicitly set options if available, otherwise falls back to model defaults
func (r *ClientRequest) getOptions() *Options {
	var opts *Options

	if r.options != nil {
		opts = r.options.Clone()
	} else {
		opts = r.model.DefaultOptions().Clone()
	}

	return opts
}

// buildRequest constructs the final Request object from the ClientRequest configuration
// Returns an error if the texts is invalid
func (r *ClientRequest) buildRequest() (*Request, error) {
	req, err := NewRequest(r.texts)
	if err != nil {
		return nil, err
	}

	req.Options = r.getOptions()
	req.Params = maps.Clone(r.params)

	return req, nil
}

// Call creates a ClientCaller to execute the moderation request
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{
		request: r,
	}
}

// ClientCaller handles the execution of moderation requests and provides various
// methods to retrieve different aspects of the moderation response
type ClientCaller struct {
	request *ClientRequest
}

// Response executes the moderation request and returns the complete response
// Returns an error if the request fails or validation fails
func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}
	return c.
		request.
		MiddlewareManager().
		BuildHandler(c.request.model).
		Call(ctx, req)
}

// Moderation executes the request and returns the first moderation result along with the full response
// Returns an error if the request fails
func (c *ClientCaller) Moderation(ctx context.Context) (*Moderation, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resp.Result().Moderation, resp, nil
}

// Moderations executes the request and returns all moderation results along with the full response
// Returns an error if the request fails
func (c *ClientCaller) Moderations(ctx context.Context) ([]*Moderation, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	moderations := make([]*Moderation, 0, len(resp.Results))
	for _, result := range resp.Results {
		moderations = append(moderations, result.Moderation)
	}
	return moderations, resp, nil
}

// Client provides a high-level interface for making moderation requests
// It maintains a default request configuration that can be cloned and customized
type Client struct {
	defaultRequest *ClientRequest
}

// NewClient creates a new Client with the specified default request configuration
// Returns an error if the request is nil
func NewClient(request *ClientRequest) (*Client, error) {
	if request == nil {
		return nil, errors.New("client request is required")
	}
	return &Client{
		defaultRequest: request,
	}, nil
}

// NewClientWithModel creates a new Client with a default request using the specified model
// Returns an error if the model is invalid
func NewClientWithModel(model Model) (*Client, error) {
	cliReq, err := NewClientRequest(model)
	if err != nil {
		return nil, err
	}
	return NewClient(cliReq)
}

// Moderate creates a new ClientRequest by cloning the default request configuration
// This is the starting point for building a customized moderation request
func (c *Client) Moderate() *ClientRequest {
	return c.
		defaultRequest.
		Clone()
}

// ModerateWithRequest creates a ClientRequest from an existing Request object
// Copies the text, options, and params from the provided request
func (c *Client) ModerateWithRequest(request *Request) *ClientRequest {
	return c.
		Moderate().
		WithTexts(request.Texts).
		WithOptions(request.Options).
		WithParams(request.Params)
}

// ModerateWithText creates a ClientRequest for moderating a single text
// This is a convenience method for simple moderation tasks
func (c *Client) ModerateWithText(text string) *ClientRequest {
	return c.
		Moderate().
		WithTexts([]string{text})
}

// ModerateWithTexts creates a ClientRequest for moderating text contents
// This is a convenience method for batch moderation tasks
func (c *Client) ModerateWithTexts(texts []string) *ClientRequest {
	return c.
		Moderate().
		WithTexts(texts)
}
