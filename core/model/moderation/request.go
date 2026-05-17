package moderation

import (
	"errors"
	"maps"
)

// Options holds per-request configuration for a moderation call.
type Options struct {
	// Model is the provider model identifier.
	Model string `json:"model"`

	// Extra carries provider-specific options unknown to this struct.
	Extra map[string]any `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("moderation.NewOptions: model id must not be empty")
	}
	return &Options{Model: model}, nil
}

func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (o *Options) Get(key string) (any, bool) {
	if o.Extra == nil {
		return nil, false
	}
	value, exists := o.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (o *Options) Set(key string, value any) {
	o.ensureExtra()
	o.Extra[key] = value
}

// Clone returns a deep copy. nil receiver yields nil.
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		Model: o.Model,
		Extra: maps.Clone(o.Extra),
	}
}

// MergeOptions clones base then applies each override left-to-right.
// Returns an error when base is nil.
func MergeOptions(base *Options, overrides ...*Options) (*Options, error) {
	if base == nil {
		return nil, errors.New("moderation.MergeOptions: base options must not be nil")
	}

	merged := base.Clone()
	for _, override := range overrides {
		if override == nil {
			continue
		}
		if override.Model != "" {
			merged.Model = override.Model
		}
		if len(override.Extra) > 0 {
			if merged.Extra == nil {
				merged.Extra = make(map[string]any, len(override.Extra))
			}
			maps.Copy(merged.Extra, override.Extra)
		}
	}
	return merged, nil
}

// Request is one moderation call: the input texts, options, and
// caller-supplied side-channel params.
type Request struct {
	// Texts is the input list. Each entry is moderated independently.
	Texts []string `json:"text,omitzero"`

	// Options carries model-specific parameters.
	Options *Options `json:"options,omitempty"`

	// Params is per-request metadata middlewares can read.
	Params map[string]any `json:"params,omitzero"`
}

// NewRequest builds a Request from texts. Returns an error when texts
// is empty.
func NewRequest(texts []string) (*Request, error) {
	if len(texts) == 0 {
		return nil, errors.New("moderation.NewRequest: texts must contain at least one entry")
	}
	return &Request{Texts: texts}, nil
}

func (r *Request) ensureParams() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

// Get returns the Params value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (r *Request) Get(key string) (any, bool) {
	if r.Params == nil {
		return nil, false
	}
	value, exists := r.Params[key]
	return value, exists
}

// Set stores value under key in Params.
func (r *Request) Set(key string, value any) {
	r.ensureParams()
	r.Params[key] = value
}
