package transcription

import (
	"errors"
	"fmt"
	"strings"

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
	return m.Extra.Set(key, value)
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
	result := &Result{Text: text, Metadata: metadata}
	if err := validateResult(result); err != nil {
		return nil, fmt.Errorf("transcription.NewResult: %w", err)
	}
	return result, nil
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
	return m.Extra.Set(key, value)
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

// NewResponse builds a [Response] from a non-nil result and metadata.
func NewResponse(result *Result, metadata *ResponseMetadata) (*Response, error) {
	response := &Response{Result: result, Metadata: metadata}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("transcription.NewResponse: %w", err)
	}
	return response, nil
}

// Validate recursively verifies transcription and response metadata.
func (r *Response) Validate() error {
	if r == nil {
		return errors.New("transcription.Response: nil response")
	}
	if err := validateResult(r.Result); err != nil {
		return fmt.Errorf("transcription.Response: result: %w", err)
	}
	if r.Metadata == nil {
		return errors.New("transcription.Response: metadata must not be nil")
	}
	if r.Metadata.Model != "" && strings.TrimSpace(r.Metadata.Model) != r.Metadata.Model {
		return errors.New("transcription.Response: metadata model must not have surrounding whitespace")
	}
	if r.Metadata.Created < 0 {
		return errors.New("transcription.Response: created must not be negative")
	}
	if err := r.Metadata.Extra.Validate(); err != nil {
		return fmt.Errorf("transcription.Response: metadata: %w", err)
	}
	return nil
}

func validateResult(result *Result) error {
	if result == nil {
		return errors.New("result must not be nil")
	}
	if result.Metadata == nil {
		return errors.New("metadata must not be nil")
	}
	if err := result.Metadata.Extra.Validate(); err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	return nil
}
