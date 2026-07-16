package speech

import (
	"fmt"
	"slices"
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
		return fmt.Errorf("speech.ResultMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("speech.ResultMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
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
	result := &Result{Audio: slices.Clone(audio), Metadata: metadata}
	if err := result.validate(); err != nil {
		return nil, fmt.Errorf("speech.NewResult: %w", err)
	}
	return result, nil
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
		return fmt.Errorf("speech.ResponseMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("speech.ResponseMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
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
	response := &Response{Result: result, Metadata: metadata}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("speech.NewResponse: %w", err)
	}
	return response, nil
}

// Validate recursively verifies audio and response metadata.
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
	if len(r.Audio) == 0 {
		return fmt.Errorf("%w: audio must not be empty", ErrInvalidResponse)
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
