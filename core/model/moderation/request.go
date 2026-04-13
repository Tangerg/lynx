package moderation

import (
	"errors"
	"maps"
)

// Options represents configuration options for moderation requests
type Options struct {
	// Model specifies the moderation model to use
	Model string `json:"model"`

	// Extra holds provider-specific options that are not part of the standard fields
	Extra map[string]any `json:"extra"`
}

// NewOptions creates a new Options instance with the specified model
// Returns an error if model is empty
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("no model provided")
	}
	return &Options{
		Model: model,
	}, nil
}

func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

func (o *Options) Get(key string) (any, bool) {
	o.ensureExtra()
	value, exists := o.Extra[key]
	return value, exists
}

func (o *Options) Set(key string, value any) {
	o.ensureExtra()
	o.Extra[key] = value
}

// Clone creates a deep copy of the Options instance
// Returns nil if the original Options is nil
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		Model: o.Model,
		Extra: maps.Clone(o.Extra),
	}
}

// MergeOptions merges multiple Options instances into a single Options
// Later options override earlier ones for conflicting fields
// Returns an error if the base options parameter is nil
func MergeOptions(options *Options, opts ...*Options) (*Options, error) {
	if options == nil {
		return nil, errors.New("options cannot be nil")
	}
	mergedOpts := options.Clone()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if opt.Model != "" {
			mergedOpts.Model = opt.Model
		}
		if len(opt.Extra) > 0 {
			maps.Copy(mergedOpts.Extra, opt.Extra)
		}
	}

	return mergedOpts, nil
}

// Request represents a moderation request containing text and configuration
type Request struct {
	// Texts is the text contents to be moderated
	Texts []string `json:"text"`

	// Options contains the moderation configuration settings
	Options *Options `json:"options"`

	// Params holds additional request-specific parameters
	Params map[string]any `json:"params"`
}

func (r *Request) ensureParams() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

func (r *Request) Get(key string) (any, bool) {
	r.ensureParams()
	value, exists := r.Params[key]
	return value, exists
}

func (r *Request) Set(key string, value any) {
	r.ensureParams()
	r.Params[key] = value
}

// NewRequest creates a new Request instance with the specified text
// Returns an error if text is empty
func NewRequest(texts []string) (*Request, error) {
	if len(texts) == 0 {
		return nil, errors.New("no texts provided")
	}
	return &Request{
		Texts: texts,
	}, nil
}
