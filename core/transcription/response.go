package transcription

import (
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
		return fmt.Errorf("transcription.ResultMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("transcription.ResultMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
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
	if err := result.validate(); err != nil {
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
		return fmt.Errorf("transcription.ResponseMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("transcription.ResponseMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
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
		return fmt.Errorf("%w: nil response", ErrInvalidResponse)
	}
	if err := r.Result.validate(); err != nil {
		return fmt.Errorf("%w: result: %w", ErrInvalidResponse, err)
	}
	if err := r.Metadata.validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (r *Result) validate() error {
	if r == nil {
		return fmt.Errorf("%w: result must not be nil", ErrInvalidResponse)
	}
	if err := r.Metadata.validate(); err != nil {
		return err
	}
	return nil
}

func (m *ResultMetadata) validate() error {
	if m == nil {
		return fmt.Errorf("%w: result metadata must not be nil", ErrInvalidResponse)
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: result metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m *ResponseMetadata) validate() error {
	if m == nil {
		return fmt.Errorf("%w: response metadata must not be nil", ErrInvalidResponse)
	}
	if m.Model != "" && strings.TrimSpace(m.Model) != m.Model {
		return fmt.Errorf("%w: response metadata model must not have surrounding whitespace", ErrInvalidResponse)
	}
	if m.Created < 0 {
		return fmt.Errorf("%w: created must not be negative", ErrInvalidResponse)
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: response metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}
