package transcription

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model"
)

type (
	Handler         = model.CallHandler[*Request, *Response]
	HandlerFunc     = model.CallHandlerFunc[*Request, *Response]
	Middleware      = model.CallMiddleware[*Request, *Response]
	MiddlewareChain = model.MiddlewareChain[*Request, *Response]
)

// NewMiddlewareChain returns an empty [MiddlewareChain] keyed to
// transcription's *Request / *Response pair. The stream side is unused
// (transcription has no stream endpoint).
func NewMiddlewareChain() MiddlewareChain {
	return model.NewMiddlewareChain[*Request, *Response]()
}

// ClientRequest is the fluent builder that turns a [Model] plus an
// audio payload into a transcription call.
type ClientRequest struct {
	model       Model
	middlewares MiddlewareChain
	options     *Options
	audio       *media.Media
	params      map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("transcription.NewClientRequest: model must not be nil")
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

// WithAudio sets the audio payload. nil is ignored.
func (r *ClientRequest) WithAudio(audio *media.Media) *ClientRequest {
	if audio != nil {
		r.audio = audio
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

// Clone returns a shallow copy of the request. The audio payload
// reference is shared (audio bytes can be large; sharing is the cheap
// default — call WithAudio explicitly if isolation is required).
func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:       r.model,
		middlewares: r.middlewares.Clone(),
		options:     r.options.Clone(),
		audio:       r.audio,
		params:      maps.Clone(r.params),
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

func (c *ClientCaller) Text(ctx context.Context) (string, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return "", nil, err
	}
	return resp.Result.Text, resp, nil
}

// Client wraps a [Model] with a sticky default [ClientRequest].
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
		return nil, errors.New("transcription.NewClientFromRequest: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

func (c *Client) Transcribe() *ClientRequest {
	return c.defaultRequest.Clone()
}

func (c *Client) TranscribeWithRequest(req *Request) *ClientRequest {
	return c.Transcribe().
		WithAudio(req.Audio).
		WithOptions(req.Options).
		WithParams(req.Params)
}

func (c *Client) TranscribeWithAudio(audio *media.Media) *ClientRequest {
	return c.Transcribe().WithAudio(audio)
}
