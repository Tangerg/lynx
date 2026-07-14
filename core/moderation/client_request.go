package moderation

import (
	"errors"
	"maps"
	"slices"
)

// ClientRequest is the fluent builder that turns a [Model] plus inputs
// and options into a moderation call. Construct one with
// [NewClientRequest] (or [Client.Moderate] which clones the client's
// default), chain WithXxx, then finish with [ClientRequest.Call].
type ClientRequest struct {
	model       Model
	middlewares MiddlewareChain
	options     *Options
	texts       []string
	params      map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("moderation.NewClientRequest: model must not be nil")
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

// WithTexts replaces the input list. Empty input is ignored. The
// slice is cloned so caller mutations don't leak into the request.
func (r *ClientRequest) WithTexts(texts []string) *ClientRequest {
	if len(texts) > 0 {
		r.texts = slices.Clone(texts)
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
		texts:       slices.Clone(r.texts),
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
	req, err := NewRequest(r.texts)
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
//	cats, _, err := client.Moderate().WithTexts([]string{"hi"}).Call().Categories(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}
