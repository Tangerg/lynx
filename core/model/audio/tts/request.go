package tts

import (
	"errors"
	"maps"
)

// Options holds per-request configuration for a TTS call.
type Options struct {
	// Model is the provider model identifier (e.g. "tts-1").
	Model string `json:"model"`

	// Voice selects the speaker profile. Provider-specific values.
	Voice string `json:"voice"`

	// ResponseFormat selects the audio container ("mp3", "wav", ...).
	ResponseFormat string `json:"format"`

	// Speed scales the playback rate. 1.0 is normal speed.
	Speed float64 `json:"speed"`

	// Extra carries provider-specific options unknown to this struct.
	Extra map[string]any `json:"extra"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("tts.NewOptions: model id must not be empty")
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
		Model:          o.Model,
		Voice:          o.Voice,
		ResponseFormat: o.ResponseFormat,
		Speed:          o.Speed,
		Extra:          maps.Clone(o.Extra),
	}
}

// MergeOptions clones base then applies each override left-to-right.
// Scalar non-zero values overwrite; the Extra map merges last-write-wins.
// Returns an error when base is nil.
func MergeOptions(base *Options, overrides ...*Options) (*Options, error) {
	if base == nil {
		return nil, errors.New("tts.MergeOptions: base options must not be nil")
	}

	merged := base.Clone()
	for _, override := range overrides {
		if override == nil {
			continue
		}
		if override.Model != "" {
			merged.Model = override.Model
		}
		if override.Voice != "" {
			merged.Voice = override.Voice
		}
		if override.ResponseFormat != "" {
			merged.ResponseFormat = override.ResponseFormat
		}
		if override.Speed != 0 {
			merged.Speed = override.Speed
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

// Request is one TTS call: the input text, options, and caller-supplied
// side-channel params.
type Request struct {
	// Text is the prompt converted to speech.
	Text string `json:"text"`

	// Options carries model-specific parameters.
	Options *Options `json:"options"`

	// Params is per-request metadata middlewares can read.
	Params map[string]any `json:"params"`
}

// NewRequest builds a Request from text. Returns an error when text
// is empty.
func NewRequest(text string) (*Request, error) {
	if text == "" {
		return nil, errors.New("tts.NewRequest: text must not be empty")
	}
	return &Request{Text: text}, nil
}
