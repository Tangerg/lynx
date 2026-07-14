package speech

import (
	"errors"
	"maps"
)

// ClientRequest is the fluent builder that turns a [Model] plus text and
// options into a TTS call. Use [ClientRequest.Call] for synchronous
// generation or [ClientRequest.Stream] for chunked output.
type ClientRequest struct {
	model       Model
	middlewares MiddlewareChain
	options     *Options
	text        string
	params      map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("tts.NewClientRequest: model must not be nil")
	}
	return &ClientRequest{model: model}, nil
}

// WithMiddlewareChain replaces the full middleware chain.
func (r *ClientRequest) WithMiddlewareChain(chain MiddlewareChain) *ClientRequest {
	r.middlewares = chain.Clone()
	return r
}

// WithCallMiddlewares replaces the call-side middleware chain.
func (r *ClientRequest) WithCallMiddlewares(middlewares ...CallMiddleware) *ClientRequest {
	r.middlewares = r.middlewares.WithCall(middlewares...)
	return r
}

// WithStreamMiddlewares replaces the stream-side middleware chain.
func (r *ClientRequest) WithStreamMiddlewares(middlewares ...StreamMiddleware) *ClientRequest {
	r.middlewares = r.middlewares.WithStream(middlewares...)
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

func (r *ClientRequest) MiddlewareChain() MiddlewareChain {
	return r.middlewares.Clone()
}

func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:       r.model,
		middlewares: r.middlewares.Clone(),
		options:     r.options.Clone(),
		text:        r.text,
		params:      maps.Clone(r.params),
	}
}

func (r *ClientRequest) resolveOptions() *Options {
	defaults := r.model.DefaultOptions()
	if r.options != nil {
		merged, err := MergeOptions(&defaults, r.options)
		if err == nil {
			return merged
		}
	}
	return defaults.Clone()
}

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
