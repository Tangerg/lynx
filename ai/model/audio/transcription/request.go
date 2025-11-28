package transcription

import (
	"errors"
	"maps"

	"github.com/Tangerg/lynx/ai/media"
)

// Options represents configuration options for audio transcription requests
type Options struct {
	// Model specifies the transcription model to use for audio-to-text conversion
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

// Request represents an audio transcription request containing audio data and configuration
type Request struct {
	// Audio is the audio media to be transcribed to text
	Audio *media.Media `json:"audio"`

	// Options contains the transcription configuration settings
	Options *Options `json:"options"`

	// Params holds additional request-specific parameters
	Params map[string]any `json:"params"`
}

// NewRequest creates a new Request instance with the specified audio media
// Returns an error if audio is nil
func NewRequest(audio *media.Media) (*Request, error) {
	if audio == nil {
		return nil, errors.New("audio cannot be nil")
	}
	return &Request{
		Audio: audio,
	}, nil
}
