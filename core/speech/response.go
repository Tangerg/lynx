package speech

import (
	"errors"

	"github.com/Tangerg/lynx/core/metadata"
)

// ResultMetadata holds per-segment metadata returned by the provider.
type ResultMetadata struct {
	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific result metadata into Extra.
func (m *ResultMetadata) Set(key string, value any) error {
	if m == nil {
		return errors.New("speech.ResultMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
}

// Result is one chunk of generated audio. For synchronous calls the
// chunk is the entire audio; for streaming calls Audio is whatever
// segment the provider just produced.
type Result struct {
	// Audio holds the encoded bytes in the format selected by
	// Request.Options.OutputFormat.
	Audio []byte `json:"audio,omitzero"`

	// Metadata carries per-chunk extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// NewResult builds a [Result]. Returns an error when audio is empty
// or metadata is nil.
func NewResult(audio []byte, metadata *ResultMetadata) (*Result, error) {
	if len(audio) == 0 {
		return nil, errors.New("speech.NewResult: audio must not be empty")
	}
	if metadata == nil {
		return nil, errors.New("speech.NewResult: metadata must not be nil")
	}
	return &Result{Audio: audio, Metadata: metadata}, nil
}

// ResponseMetadata holds response-level metadata for a TTS call.
type ResponseMetadata struct {
	// Model is the model name actually served.
	Model string `json:"model"`

	// Created is the provider-reported creation time, Unix seconds.
	Created int64 `json:"created"`

	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific response metadata into Extra.
func (m *ResponseMetadata) Set(key string, value any) error {
	if m == nil {
		return errors.New("speech.ResponseMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
}

// Response is one TTS call's audio output plus shared metadata. For
// synchronous calls Result holds the entire audio; for streaming calls
// each chunk yields a Response with the just-produced segment in Result.
type Response struct {
	// Result holds the generated audio. Non-nil after [NewResponse].
	Result *Result `json:"result,omitempty"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from a non-nil result and metadata.
func NewResponse(result *Result, metadata *ResponseMetadata) (*Response, error) {
	if result == nil {
		return nil, errors.New("speech.NewResponse: result must not be nil")
	}
	if metadata == nil {
		return nil, errors.New("speech.NewResponse: metadata must not be nil")
	}
	return &Response{Result: result, Metadata: metadata}, nil
}
