package transcription

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model"
)

// Type aliases threading transcription's *Request / *Response into the
// generic [model] handler/middleware machinery.
type (
	Handler           = model.CallHandler[*Request, *Response]
	HandlerFunc       = model.CallHandlerFunc[*Request, *Response]
	Middleware        = model.CallMiddleware[*Request, *Response]
	MiddlewareManager = model.MiddlewareManager[*Request, *Response]
)

// NewMiddlewareManager returns an empty [MiddlewareManager] keyed to
// transcription's *Request / *Response pair. The stream side is unused
// (transcription has no stream endpoint).
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewMiddlewareManager[*Request, *Response]()
}

// ClientRequest is the fluent builder that turns a [Model] plus an
// audio payload into a transcription call.
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	audio             *media.Media
	params            map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("transcription.NewClientRequest: model must not be nil")
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

// WithAudio sets the audio payload. nil is ignored.
func (r *ClientRequest) WithAudio(audio *media.Media) *ClientRequest {
	if audio != nil {
		r.audio = audio
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

// Clone returns a shallow copy of the request. The audio payload
// reference is shared (audio bytes can be large; sharing is the cheap
// default — call WithAudio explicitly if isolation is required).
func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:             r.model,
		middlewareManager: r.middlewareManager.Clone(),
		options:           r.options.Clone(),
		audio:             r.audio,
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
	req, err := NewRequest(r.audio)
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
//	text, _, err := client.Transcribe().WithAudio(m).Call().Text(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}

// ClientCaller drives the synchronous transcription path.
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

// Text runs the call and returns the first transcribed segment alongside
// the full response.
func (c *ClientCaller) Text(ctx context.Context) (string, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return "", nil, err
	}
	return resp.Result().Text, resp, nil
}

// Client wraps a [Model] with a sticky default [ClientRequest].
type Client struct {
	defaultRequest *ClientRequest
}

// NewClient wraps an existing [ClientRequest] as a sticky default.
// Returns an error when request is nil.
func NewClient(request *ClientRequest) (*Client, error) {
	if request == nil {
		return nil, errors.New("transcription.NewClient: request must not be nil")
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

// Transcribe returns a fresh clone of the default request.
func (c *Client) Transcribe() *ClientRequest {
	return c.defaultRequest.Clone()
}

// TranscribeWithRequest seeds a clone with the audio, options, and
// params from req.
func (c *Client) TranscribeWithRequest(req *Request) *ClientRequest {
	return c.Transcribe().
		WithAudio(req.Audio).
		WithOptions(req.Options).
		WithParams(req.Params)
}

// TranscribeWithAudio is the most common shortcut.
func (c *Client) TranscribeWithAudio(audio *media.Media) *ClientRequest {
	return c.Transcribe().WithAudio(audio)
}
