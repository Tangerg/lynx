package transcription

import (
	"errors"
	"maps"

	"github.com/Tangerg/lynx/core/media"
)

// Options holds per-request configuration for a transcription call.
type Options struct {
	// Model is the provider model identifier (e.g. "whisper-1").
	Model string `json:"model"`

	// Extra carries provider-specific options unknown to this struct.
	Extra map[string]any `json:"extra"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("transcription.NewOptions: model id must not be empty")
	}
	return &Options{Model: model}, nil
}

func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag.
func (o *Options) Get(key string) (any, bool) {
	o.ensureExtra()
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
		return nil, errors.New("transcription.MergeOptions: base options must not be nil")
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

// Request is one transcription call: the audio payload, options, and
// caller-supplied side-channel params.
type Request struct {
	// Audio carries the audio bytes (or URL) to transcribe.
	Audio *media.Media `json:"audio"`

	// Options carries model-specific parameters.
	Options *Options `json:"options"`

	// Params is per-request metadata middlewares can read.
	Params map[string]any `json:"params"`
}

// NewRequest builds a Request from an audio payload. Returns an error
// when audio is nil.
func NewRequest(audio *media.Media) (*Request, error) {
	if audio == nil {
		return nil, errors.New("transcription.NewRequest: audio must not be nil")
	}
	return &Request{Audio: audio}, nil
}
