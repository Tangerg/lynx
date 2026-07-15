package transcription

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
		return errors.New("transcription.ResultMetadata.Set: nil receiver")
	}
	return setExtra(&m.Extra, key, value)
}

// Result is one transcription segment.
type Result struct {
	// Text is the transcribed text. Empty is allowed for partial /
	// silence segments.
	Text string `json:"text"`

	// Metadata carries per-segment extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// NewResult builds a [Result]. Text may be empty; metadata is required.
func NewResult(text string, metadata *ResultMetadata) (*Result, error) {
	if metadata == nil {
		return nil, errors.New("transcription.NewResult: metadata must not be nil")
	}
	return &Result{Text: text, Metadata: metadata}, nil
}

// ResponseMetadata holds response-level metadata for a transcription call.
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
		return errors.New("transcription.ResponseMetadata.Set: nil receiver")
	}
	return setExtra(&m.Extra, key, value)
}

// Response is one transcription call's output plus shared metadata.
// Providers that emit per-segment timing (Whisper verbose_json) should
// stash the segment array under Result.Metadata.Extra; the top-level
// Result holds the merged transcript text.
type Response struct {
	// Result holds the transcribed text. Non-nil after [NewResponse].
	Result *Result `json:"result,omitempty"`

	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

func NewResponse(result *Result, metadata *ResponseMetadata) (*Response, error) {
	if result == nil {
		return nil, errors.New("transcription.NewResponse: result must not be nil")
	}
	if metadata == nil {
		return nil, errors.New("transcription.NewResponse: metadata must not be nil")
	}
	return &Response{Result: result, Metadata: metadata}, nil
}
