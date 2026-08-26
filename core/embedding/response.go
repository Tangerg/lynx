package embedding

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

// OutputMetadata holds provider-specific per-embedding metadata.
type OutputMetadata struct {
	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific output metadata into Extra.
func (m *OutputMetadata) Set(key string, value any) error {
	if m == nil {
		return fmt.Errorf("embedding.OutputMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("embedding.OutputMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m *OutputMetadata) validate() error {
	if m == nil {
		return fmt.Errorf("%w: output metadata must not be nil", ErrInvalidResponse)
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: output metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m OutputMetadata) MarshalJSON() ([]byte, error) {
	if err := (&m).validate(); err != nil {
		return nil, err
	}
	type wireOutputMetadata OutputMetadata
	return json.Marshal(wireOutputMetadata(m))
}

func (m *OutputMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
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
	*m = candidate
	return nil
}

// Output is one embedding plus its metadata.
type Output struct {
	// Embedding is the vector representation of the input.
	Embedding []float64 `json:"embedding"`

	// Metadata carries provider-specific per-output extras.
	Metadata *OutputMetadata `json:"metadata,omitempty"`
}

// NewOutput builds a [Output]. Returns an error when the embedding is
// empty or metadata is nil.
func NewOutput(embedding []float64, metadata *OutputMetadata) (*Output, error) {
	output := &Output{Embedding: slices.Clone(embedding), Metadata: metadata}
	if err := output.Validate(); err != nil {
		return nil, fmt.Errorf("embedding.NewOutput: %w", err)
	}
	return output, nil
}

// Validate verifies the embedding vector and output metadata.
func (r *Output) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: output must not be nil", ErrInvalidResponse)
	}
	if len(r.Embedding) == 0 {
		return fmt.Errorf("%w: embedding vector must not be empty", ErrInvalidResponse)
	}
	for i, value := range r.Embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%w: embedding[%d] must be finite", ErrInvalidResponse, i)
		}
	}
	if err := r.Metadata.validate(); err != nil {
		return err
	}
	return nil
}

func (r Output) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireOutput Output
	return json.Marshal(wireOutput(r))
}

func (r *Output) UnmarshalJSON(data []byte) error {
	if r == nil {
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
	*r = candidate
	return nil
}

// Usage records the token consumption an embedding request reported back.
// Embedding is input-only — there is no completion, reasoning, or cache
// dimension — so a single count is the whole story. Providers that report
// a "total" figure map it here: for embeddings every token is input.
type Usage struct {
	// InputTokens are tokens consumed embedding the inputs.
	InputTokens int64 `json:"input_tokens"`
}

func (u Usage) validate() error {
	if u.InputTokens < 0 {
		return fmt.Errorf("%w: input tokens must not be negative", ErrInvalidResponse)
	}
	return nil
}

func (u Usage) MarshalJSON() ([]byte, error) {
	if err := u.validate(); err != nil {
		return nil, err
	}
	type wireUsage Usage
	return json.Marshal(wireUsage(u))
}

func (u *Usage) UnmarshalJSON(data []byte) error {
	if u == nil {
		return fmt.Errorf("%w: nil Usage receiver", ErrInvalidResponse)
	}
	type wireUsage Usage
	var decoded wireUsage
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode usage: %w", ErrInvalidResponse, err)
	}
	candidate := Usage(decoded)
	if err := candidate.validate(); err != nil {
		return err
	}
	*u = candidate
	return nil
}

// ResponseMetadata holds response-level metadata: the model actually
// used, token usage, creation time, and provider extras.
type ResponseMetadata struct {
	// Model is the model name actually served.
	Model string `json:"model"`

	// Usage breaks down token consumption. nil means the provider did not
	// report usage.
	Usage *Usage `json:"usage,omitempty"`

	// Created is the provider-reported creation time, Unix seconds.
	Created int64 `json:"created"`

	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific response metadata into Extra.
func (m *ResponseMetadata) Set(key string, value any) error {
	if m == nil {
		return fmt.Errorf("embedding.ResponseMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("embedding.ResponseMetadata.Set: %w: %w", ErrInvalidResponse, err)
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
	if m.Usage != nil {
		if err := m.Usage.validate(); err != nil {
			return err
		}
	}
	if m.Created < 0 {
		return fmt.Errorf("%w: created must not be negative", ErrInvalidResponse)
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: response metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m ResponseMetadata) MarshalJSON() ([]byte, error) {
	if err := (&m).validate(); err != nil {
		return nil, err
	}
	type wireResponseMetadata ResponseMetadata
	return json.Marshal(wireResponseMetadata(m))
}

func (m *ResponseMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
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
	*m = candidate
	return nil
}

// Response is the full embedding output: one [*Output] per input plus
// shared response metadata.
type Response struct {
	// Outputs holds one entry per input text, in the same order.
	Outputs []*Output `json:"outputs,omitzero"`

	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from at least one output and a
// non-nil metadata.
func NewResponse(outputs []*Output, metadata *ResponseMetadata) (*Response, error) {
	response := &Response{Outputs: slices.Clone(outputs), Metadata: metadata}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("embedding.NewResponse: %w", err)
	}
	return response, nil
}

// Validate recursively verifies the response and its vector, metadata, and
// usage invariants.
func (r *Response) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: nil response", ErrInvalidResponse)
	}
	if len(r.Outputs) == 0 {
		return fmt.Errorf("%w: at least one output is required", ErrInvalidResponse)
	}
	dimensions := -1
	for i, output := range r.Outputs {
		if err := output.Validate(); err != nil {
			return fmt.Errorf("%w: outputs[%d]: %w", ErrInvalidResponse, i, err)
		}
		if dimensions < 0 {
			dimensions = len(output.Embedding)
		} else if len(output.Embedding) != dimensions {
			return fmt.Errorf("%w: outputs[%d]: dimensions = %d, want %d", ErrInvalidResponse, i, len(output.Embedding), dimensions)
		}
	}
	if err := r.Metadata.validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

// First returns the first embedding — the common single-input shortcut.
// Returns nil when Outputs is empty.
func (r *Response) First() *Output {
	if r == nil || len(r.Outputs) == 0 {
		return nil
	}
	return r.Outputs[0]
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
