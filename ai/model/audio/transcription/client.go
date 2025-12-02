package transcription

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/ai/model"
)

type Handler = model.CallHandler[*Request, *Response]
type HandlerFunc = model.CallHandlerFunc[*Request, *Response]
type Middleware = model.CallMiddleware[*Request, *Response]
type MiddlewareManager = model.CallMiddlewareManager[*Request, *Response]

// NewMiddlewareManager creates a new middleware manager for transcription requests
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewCallMiddlewareManager[*Request, *Response, *Request, *Response]()
}

// ClientRequest represents a fluent builder for constructing audio transcription requests
// It allows configuring the model, middlewares, options, audio data, and additional parameters
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	audio             *media.Media
	params            map[string]any
}

// NewClientRequest creates a new ClientRequest with the specified transcription model
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

// WithAudio sets the audio media to be transcribed
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithAudio(audio *media.Media) *ClientRequest {
	if audio != nil {
		r.audio = audio
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
// Note: The audio media reference is shared to conserve memory
func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:             r.model,
		middlewareManager: r.middlewareManager.Clone(),
		options:           r.options.Clone(),
		audio:             r.audio, // use same audio file reference to save memory
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
// Returns an error if the audio is invalid
func (r *ClientRequest) buildRequest() (*Request, error) {
	req, err := NewRequest(r.audio)
	if err != nil {
		return nil, err
	}

	req.Options = r.getOptions()
	req.Params = maps.Clone(r.params)

	return req, nil
}

// Call creates a ClientCaller to execute the transcription request synchronously
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{
		request: r,
	}
}

// ClientCaller handles the synchronous execution of transcription requests and provides
// various methods to retrieve different aspects of the transcription response
type ClientCaller struct {
	request *ClientRequest
}

// Response executes the transcription request and returns the complete response
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

// Text executes the request and returns the first transcribed text along with the full response
// Returns an error if the request fails
func (c *ClientCaller) Text(ctx context.Context) (string, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return "", nil, err
	}
	return resp.Result().Text, resp, nil
}

// Client provides a high-level interface for making audio transcription requests
// It maintains a default request configuration that can be cloned and customized
// Supports synchronous audio-to-text transcription operations
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

// Transcribe creates a new ClientRequest by cloning the default request configuration
// This is the starting point for building a customized transcription request
func (c *Client) Transcribe() *ClientRequest {
	return c.
		defaultRequest.
		Clone()
}

// TranscribeWithRequest creates a ClientRequest from an existing Request object
// Copies the audio, options, and params from the provided request
func (c *Client) TranscribeWithRequest(request *Request) *ClientRequest {
	return c.
		Transcribe().
		WithAudio(request.Audio).
		WithOptions(request.Options).
		WithParams(request.Params)
}

// TranscribeWithAudio creates a ClientRequest for transcribing a single audio media
// This is a convenience method for simple transcription tasks
func (c *Client) TranscribeWithAudio(audio *media.Media) *ClientRequest {
	return c.
		Transcribe().
		WithAudio(audio)
}
