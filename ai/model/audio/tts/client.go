package tts

import (
	"context"
	"errors"
	"iter"
	"maps"

	"github.com/Tangerg/lynx/ai/model"
)

type CallHandler = model.CallHandler[*Request, *Response]
type StreamHandler = model.StreamHandler[*Request, *Response]
type CallHandlerFunc = model.CallHandlerFunc[*Request, *Response]
type StreamHandlerFunc = model.StreamHandlerFunc[*Request, *Response]
type CallMiddleware = model.CallMiddleware[*Request, *Response]
type StreamMiddleware = model.StreamMiddleware[*Request, *Response]
type MiddlewareManager = model.MiddlewareManager[*Request, *Response, *Request, *Response]

// NewMiddlewareManager creates a new middleware manager for TTS requests
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewMiddlewareManager[*Request, *Response, *Request, *Response]()
}

// ClientRequest represents a fluent builder for constructing text-to-speech requests
// It allows configuring the model, middlewares, options, text content, and additional parameters
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	text              string
	params            map[string]any
}

// NewClientRequest creates a new ClientRequest with the specified TTS model
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
// Accepts both CallMiddleware and StreamMiddleware types
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithMiddlewares(middlewares ...any) *ClientRequest {
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

// WithText sets the text content to be converted to speech
// Returns the ClientRequest for method chaining
func (r *ClientRequest) WithText(text string) *ClientRequest {
	if text != "" {
		r.text = text
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
		text:              r.text,
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
// Returns an error if the text is invalid
func (r *ClientRequest) buildRequest() (*Request, error) {
	req, err := NewRequest(r.text)
	if err != nil {
		return nil, err
	}

	req.Options = r.getOptions()
	req.Params = maps.Clone(r.params)

	return req, nil
}

// Call creates a ClientCaller to execute the TTS request synchronously
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{
		request: r,
	}
}

// Stream creates a ClientStreamer to execute the TTS request in streaming mode
func (r *ClientRequest) Stream() *ClientStreamer {
	return &ClientStreamer{
		request: r,
	}
}

// ClientCaller handles the synchronous execution of TTS requests and provides various
// methods to retrieve different aspects of the speech generation response
type ClientCaller struct {
	request *ClientRequest
}

// Response executes the TTS request and returns the complete response
// Returns an error if the request fails or validation fails
func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}
	return c.
		request.
		MiddlewareManager().
		BuildCallHandler(c.request.model).
		Call(ctx, req)
}

// Speech executes the request and returns the first speech audio data along with the full response
// Returns an error if the request fails
func (c *ClientCaller) Speech(ctx context.Context) ([]byte, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resp.Result().Speech, resp, nil
}

// ClientStreamer handles the streaming execution of TTS requests and provides
// iterator-based access to progressive speech generation responses
type ClientStreamer struct {
	request *ClientRequest
}

// stream executes the streaming request and returns an iterator of responses
func (s *ClientStreamer) stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return s.
		request.
		MiddlewareManager().
		BuildStreamHandler(s.request.model).
		Stream(ctx, req)
}

// Response executes the TTS request in streaming mode and returns an iterator of responses
// Each iteration yields a response chunk or an error
func (s *ClientStreamer) Response(ctx context.Context) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		req, err := s.request.buildRequest()
		if err != nil {
			yield(nil, err)
			return
		}

		for resp, streamErr := range s.stream(ctx, req) {
			if streamErr != nil {
				yield(nil, streamErr)
				return
			}

			if !yield(resp, nil) {
				return
			}
		}
	}
}

// Speech executes the request in streaming mode and returns an iterator of speech audio chunks
// Each iteration yields an audio data chunk or an error
func (s *ClientStreamer) Speech(ctx context.Context) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for resp, err := range s.Response(ctx) {
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(resp.Result().Speech, nil) {
				return
			}
		}
	}
}

// Client provides a high-level interface for making text-to-speech requests
// It maintains a default request configuration that can be cloned and customized
// Supports both synchronous and streaming speech generation
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

// Synthesize creates a new ClientRequest by cloning the default request configuration
// This is the starting point for building a customized TTS request
func (c *Client) Synthesize() *ClientRequest {
	return c.
		defaultRequest.
		Clone()
}

// SynthesizeWithRequest creates a ClientRequest from an existing Request object
// Copies the text, options, and params from the provided request
func (c *Client) SynthesizeWithRequest(request *Request) *ClientRequest {
	return c.
		Synthesize().
		WithText(request.Text).
		WithOptions(request.Options).
		WithParams(request.Params)
}

// SynthesizeWithText creates a ClientRequest for generating speech from a single text input
// This is a convenience method for simple TTS tasks
func (c *Client) SynthesizeWithText(text string) *ClientRequest {
	return c.
		Synthesize().
		WithText(text)
}
