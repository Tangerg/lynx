package image

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

// NewMiddlewareManager creates a new middleware manager for image generation
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewCallMiddlewareManager[*Request, *Response]()
}

// ClientRequest represents a fluent builder for constructing image generation requests
// It encapsulates the model, options, prompt, and middleware configuration
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	prompt            string
	params            map[string]any
}

// NewClientRequest creates a new ClientRequest with the specified model
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

// WithOptions sets the generation options for the request
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithOptions(options *Options) *ClientRequest {
	if options != nil {
		r.options = options
	}
	return r
}

// WithPrompt sets the prompt text for image generation
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithPrompt(prompt string) *ClientRequest {
	if prompt != "" {
		r.prompt = prompt
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

// MiddlewareManager returns the middleware manager, creating one if it doesn't exist
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
		prompt:            r.prompt,
		params:            maps.Clone(r.params),
	}
}

// getOptions returns the options to use for the request
// If custom options are set, returns a clone of them; otherwise returns the model's default options
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
// Returns an error if the prompt is invalid
func (r *ClientRequest) buildRequest() (*Request, error) {
	req, err := NewRequest(r.prompt)
	if err != nil {
		return nil, err
	}

	req.Options = r.getOptions()
	req.Params = maps.Clone(r.params)

	return req, nil
}

// Call creates a ClientCaller to execute the image generation request
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{
		request: r,
	}
}

// ClientCaller handles the execution of image generation requests
type ClientCaller struct {
	request *ClientRequest
}

// Response executes the image generation request and returns the full response
// Returns an error if request building or execution fails
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

// Image executes the request and returns the first generated image along with the full response
// Returns an error if the request fails
func (c *ClientCaller) Image(ctx context.Context) (*Image, *Response, error) {
	response, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return response.Result().Image, response, nil
}

// Images executes the request and returns all generated images along with the full response
// Returns an error if the request fails
func (c *ClientCaller) Images(ctx context.Context) ([]*Image, *Response, error) {
	response, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	images := make([]*Image, 0, len(response.Results))
	for _, result := range response.Results {
		images = append(images, result.Image)
	}
	return images, response, nil
}

// Client provides a high-level interface for image generation with a default request configuration
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

// NewClientWithModel creates a new Client with a model, using default request configuration
// Returns an error if the model is invalid
func NewClientWithModel(model Model) (*Client, error) {
	request, err := NewClientRequest(model)
	if err != nil {
		return nil, err
	}
	return NewClient(request)
}

// Generate creates a new ClientRequest by cloning the default request
// This allows customization without modifying the default configuration
func (c *Client) Generate() *ClientRequest {
	return c.
		defaultRequest.
		Clone()
}

// GeneratePrompt creates a new ClientRequest with the specified prompt
// Clones the default request and sets the prompt
func (c *Client) GeneratePrompt(prompt string) *ClientRequest {
	return c.
		defaultRequest.
		Clone().
		WithPrompt(prompt)
}

// GenerateRequest creates a new ClientRequest based on an existing Request object
// Clones the default request and applies the prompt, options, and params from the provided request
func (c *Client) GenerateRequest(request *Request) *ClientRequest {
	return c.
		defaultRequest.
		Clone().
		WithPrompt(request.Prompt).
		WithOptions(request.Options).
		WithParams(request.Params)
}
