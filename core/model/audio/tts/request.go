package tts

import (
	"errors"
	"maps"
)

// Options represents configuration options for text-to-speech generation requests
type Options struct {
	// Model specifies the TTS model to use for speech generation
	Model string `json:"model"`

	// Voice specifies the voice profile or speaker to use for synthesis
	Voice string `json:"voice"`

	// ResponseFormat specifies the audio output format (e.g., "mp3", "wav", "opus")
	ResponseFormat string `json:"format"`

	// Speed controls the speech of the generated audio
	Speed float64 `json:"speed"`

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
		if opt.Voice != "" {
			mergedOpts.Voice = opt.Voice
		}
		if opt.ResponseFormat != "" {
			mergedOpts.ResponseFormat = opt.ResponseFormat
		}
		if opt.Speed != 0 {
			mergedOpts.Speed = opt.Speed
		}
		if len(opt.Extra) > 0 {
			maps.Copy(mergedOpts.Extra, opt.Extra)
		}
	}

	return mergedOpts, nil
}

// Request represents a text-to-speech generation request containing text and configuration
type Request struct {
	// Text is the text content to be converted to speech
	Text string `json:"text"`

	// Options contains the TTS generation configuration settings
	Options *Options `json:"options"`

	// Params holds additional request-specific parameters
	Params map[string]any `json:"params"`
}

// NewRequest creates a new Request instance with the specified text
// Returns an error if text is empty
func NewRequest(text string) (*Request, error) {
	if text == "" {
		return nil, errors.New("text text cannot be empty")
	}
	return &Request{
		Text: text,
	}, nil
}
