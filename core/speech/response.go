package speech

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/scope/core/metadata"
)

// OutputMetadata holds per-segment metadata returned by the provider.
type OutputMetadata struct {
	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific output metadata into Extra.
func (o *OutputMetadata) Set(key string, value any) error {
	if o == nil {
		return fmt.Errorf("speech.OutputMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := o.Extra.Set(key, value); err != nil {
		return fmt.Errorf("speech.OutputMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (o *OutputMetadata) validate() error {
	if o == nil {
		return fmt.Errorf("%w: output metadata must not be nil", ErrInvalidResponse)
	}
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: output metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (o OutputMetadata) MarshalJSON() ([]byte, error) {
	if err := (&o).validate(); err != nil {
		return nil, err
	}
	type wireOutputMetadata OutputMetadata
	return json.Marshal(wireOutputMetadata(o))
}

func (o *OutputMetadata) UnmarshalJSON(data []byte) error {
	if o == nil {
		return fmt.Errorf("%w: nil OutputMetadata receiver", ErrInvalidResponse)
	}
	type wireOutputMetadata OutputMetadata
	var decoded wireOutputMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode output metadata: %w", ErrInvalidResponse, err)
	}
	candidate := OutputMetadata(decoded)
	if err := candidate.validate(); err != nil {
		return err
	}
	*o = candidate
	return nil
}

// Output is one chunk of generated audio. For synchronous calls the
// chunk is the entire audio; for streaming calls Audio is whatever
// segment the provider just produced.
type Output struct {
	// Audio holds the encoded bytes in the format selected by
	// Request.Options.OutputFormat.
	Audio []byte `json:"audio,omitzero"`

	// Metadata carries per-chunk extras.
	Metadata *OutputMetadata `json:"metadata,omitempty"`
}

// NewOutput builds a [Output]. Returns an error when audio is empty
// or metadata is nil.
func NewOutput(audio []byte, metadata *OutputMetadata) (*Output, error) {
	output := &Output{Audio: slices.Clone(audio), Metadata: metadata}
	if err := output.Validate(); err != nil {
		return nil, fmt.Errorf("speech.NewOutput: %w", err)
	}
	return output, nil
}

// Validate verifies audio content and output metadata.
func (o *Output) Validate() error {
	if o == nil {
		return fmt.Errorf("%w: output must not be nil", ErrInvalidResponse)
	}
	if len(o.Audio) == 0 {
		return fmt.Errorf("%w: audio must not be empty", ErrInvalidResponse)
	}
	if err := o.Metadata.validate(); err != nil {
		return err
	}
	return nil
}

func (o Output) MarshalJSON() ([]byte, error) {
	if err := (&o).Validate(); err != nil {
		return nil, err
	}
	type wireOutput Output
	return json.Marshal(wireOutput(o))
}

func (o *Output) UnmarshalJSON(data []byte) error {
	if o == nil {
		return fmt.Errorf("%w: nil Output receiver", ErrInvalidResponse)
	}
	type wireOutput Output
	var decoded wireOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode output: %w", ErrInvalidResponse, err)
	}
	candidate := Output(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*o = candidate
	return nil
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
func (r *ResponseMetadata) Set(key string, value any) error {
	if r == nil {
		return fmt.Errorf("speech.ResponseMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := r.Extra.Set(key, value); err != nil {
		return fmt.Errorf("speech.ResponseMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (r *ResponseMetadata) validate() error {
	if r == nil {
		return fmt.Errorf("%w: response metadata must not be nil", ErrInvalidResponse)
	}
	if r.Model != "" && strings.TrimSpace(r.Model) != r.Model {
		return fmt.Errorf("%w: response metadata model must not have surrounding whitespace", ErrInvalidResponse)
	}
	if r.Created < 0 {
		return fmt.Errorf("%w: created must not be negative", ErrInvalidResponse)
	}
	if err := r.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: response metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (r ResponseMetadata) MarshalJSON() ([]byte, error) {
	if err := (&r).validate(); err != nil {
		return nil, err
	}
	type wireResponseMetadata ResponseMetadata
	return json.Marshal(wireResponseMetadata(r))
}

func (r *ResponseMetadata) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil ResponseMetadata receiver", ErrInvalidResponse)
	}
	type wireResponseMetadata ResponseMetadata
	var decoded wireResponseMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode response metadata: %w", ErrInvalidResponse, err)
	}
	candidate := ResponseMetadata(decoded)
	if err := candidate.validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
}

// Response is one TTS call's audio output plus shared metadata. For
// synchronous calls Output holds the entire audio; for streaming calls
// each chunk yields a Response with the just-produced segment in Output.
type Response struct {
	// Output holds the generated audio. Non-nil after [NewResponse].
	Output *Output `json:"output,omitempty"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from a non-nil output and metadata.
func NewResponse(output *Output, metadata *ResponseMetadata) (*Response, error) {
	response := &Response{Output: output, Metadata: metadata}
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
	if err := r.Output.Validate(); err != nil {
		return fmt.Errorf("%w: output: %w", ErrInvalidResponse, err)
	}
	if err := r.Metadata.validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (r Response) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireResponse Response
	return json.Marshal(wireResponse(r))
}

func (r *Response) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil Response receiver", ErrInvalidResponse)
	}
	type wireResponse Response
	var decoded wireResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode response: %w", ErrInvalidResponse, err)
	}
	candidate := Response(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
}
