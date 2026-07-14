package image

import (
	"errors"
	"maps"
)

// ClientRequest is the fluent builder that turns a [Model] plus a
// prompt and options into an image-generation call. Construct one with
// [NewClientRequest] (or [Client.Generate] which clones the client's
// default), chain WithXxx, then finish with [ClientRequest.Call].
type ClientRequest struct {
	model       Model
	middlewares MiddlewareChain
	options     *Options
	prompt      string
	params      map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("image.NewClientRequest: model must not be nil")
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

// WithPrompt sets the prompt text. Empty input is ignored.
func (r *ClientRequest) WithPrompt(prompt string) *ClientRequest {
	if prompt != "" {
		r.prompt = prompt
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
		prompt:      r.prompt,
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
	req, err := NewRequest(r.prompt)
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
//	img, _, err := client.Generate().WithPrompt("a duck").Call().Image(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}
