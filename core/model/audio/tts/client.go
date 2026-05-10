package tts

import (
	"context"
	"errors"
	"iter"
	"maps"

	"github.com/Tangerg/lynx/core/model"
)

// Type aliases threading TTS's *Request / *Response into the generic
// [model] handler/middleware machinery. Both call and stream sides are
// exposed; concrete providers may implement either or both.
type (
	CallHandler       = model.CallHandler[*Request, *Response]
	StreamHandler     = model.StreamHandler[*Request, *Response]
	CallHandlerFunc   = model.CallHandlerFunc[*Request, *Response]
	StreamHandlerFunc = model.StreamHandlerFunc[*Request, *Response]
	CallMiddleware    = model.CallMiddleware[*Request, *Response]
	StreamMiddleware  = model.StreamMiddleware[*Request, *Response]
	MiddlewareManager = model.MiddlewareManager[*Request, *Response]
)

// NewMiddlewareManager returns an empty [MiddlewareManager] keyed to
// TTS's *Request / *Response pair.
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewMiddlewareManager[*Request, *Response]()
}

// ClientRequest is the fluent builder that turns a [Model] plus text and
// options into a TTS call. Use [ClientRequest.Call] for synchronous
// generation or [ClientRequest.Stream] for chunked output.
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	text              string
	params            map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("tts.NewClientRequest: model must not be nil")
	}
	return &ClientRequest{model: model}, nil
}

// WithMiddlewares replaces the entire middleware chain. Accepts both
// call and stream middlewares — type assertion routes each to the
// matching chain.
func (r *ClientRequest) WithMiddlewares(middlewares ...any) *ClientRequest {
	if len(middlewares) > 0 {
		r.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewares...)
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

// WithText sets the prompt text. Empty input is ignored.
func (r *ClientRequest) WithText(text string) *ClientRequest {
	if text != "" {
		r.text = text
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
		text:              r.text,
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
	req, err := NewRequest(r.text)
	if err != nil {
		return nil, err
	}
	req.Options = r.resolveOptions()
	req.Params = maps.Clone(r.params)
	return req, nil
}

// Call returns a [ClientCaller] for synchronous generation.
//
// Example:
//
//	audio, _, err := client.Synthesize().WithText("hi").Call().Speech(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}

// Stream returns a [ClientStreamer] for incremental generation.
//
// Example:
//
//	for chunk, err := range client.Synthesize().WithText("hi").Stream().Speech(ctx) {
//	    if err != nil { return err }
//	    write(chunk)
//	}
func (r *ClientRequest) Stream() *ClientStreamer {
	return &ClientStreamer{request: r}
}

// ClientCaller drives the synchronous TTS path.
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

// Speech runs the call and returns the first audio chunk alongside the
// full response.
func (c *ClientCaller) Speech(ctx context.Context) ([]byte, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resp.Result().Speech, resp, nil
}

// ClientStreamer drives the streaming TTS path.
type ClientStreamer struct {
	request *ClientRequest
}

// stream feeds the request through the middleware chain into the model.
func (s *ClientStreamer) stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return s.request.
		MiddlewareManager().
		BuildStreamHandler(s.request.model).
		Stream(ctx, req)
}

// Response yields full [*Response] chunks as they arrive.
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

// Speech yields just the audio bytes — convenient when the caller wants
// to pipe directly to a player or file.
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

// Client wraps a [Model] with a sticky default [ClientRequest], so each
// [Client.Synthesize] call clones a pre-configured starting point.
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
		return nil, errors.New("tts.NewClientFromRequest: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

// Synthesize returns a fresh clone of the default request.
func (c *Client) Synthesize() *ClientRequest {
	return c.defaultRequest.Clone()
}

// SynthesizeWithRequest seeds a clone with the text, options, and
// params from req.
func (c *Client) SynthesizeWithRequest(req *Request) *ClientRequest {
	return c.Synthesize().
		WithText(req.Text).
		WithOptions(req.Options).
		WithParams(req.Params)
}

// SynthesizeWithText is the most common shortcut.
func (c *Client) SynthesizeWithText(text string) *ClientRequest {
	return c.Synthesize().WithText(text)
}
