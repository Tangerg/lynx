package embedding

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

// ResultMetadata holds provider-specific per-embedding metadata.
type ResultMetadata struct {
	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific result metadata into Extra.
func (m *ResultMetadata) Set(key string, value any) error {
	if m == nil {
		return fmt.Errorf("embedding.ResultMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("embedding.ResultMetadata.Set: %w: %w", ErrInvalidResponse, err)
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

func (m ResultMetadata) MarshalJSON() ([]byte, error) {
	if err := (&m).validate(); err != nil {
		return nil, err
	}
	type wireResultMetadata ResultMetadata
	return json.Marshal(wireResultMetadata(m))
}

func (m *ResultMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
		return fmt.Errorf("%w: nil ResultMetadata receiver", ErrInvalidResponse)
	}
	type wireResultMetadata ResultMetadata
	var decoded wireResultMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode result metadata: %w", ErrInvalidResponse, err)
	}
	candidate := ResultMetadata(decoded)
	if err := candidate.validate(); err != nil {
		return err
	}
	*m = candidate
	return nil
}

// Result is one embedding plus its metadata.
type Result struct {
	// Embedding is the vector representation of the input.
	Embedding []float64 `json:"embedding"`

	// Metadata carries provider-specific per-result extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// NewResult builds a [Result]. Returns an error when the embedding is
// empty or metadata is nil.
func NewResult(embedding []float64, metadata *ResultMetadata) (*Result, error) {
	result := &Result{Embedding: slices.Clone(embedding), Metadata: metadata}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("embedding.NewResult: %w", err)
	}
	return result, nil
}

// Validate verifies the embedding vector and result metadata.
func (r *Result) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: result must not be nil", ErrInvalidResponse)
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

func (r Result) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireResult Result
	return json.Marshal(wireResult(r))
}

func (r *Result) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil Result receiver", ErrInvalidResponse)
	}
	type wireResult Result
	var decoded wireResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode result: %w", ErrInvalidResponse, err)
	}
	candidate := Result(decoded)
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

// Response is the full embedding result: one [*Result] per input plus
// shared response metadata.
type Response struct {
	// Results holds one entry per input text, in the same order.
	Results []*Result `json:"results,omitzero"`

	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from at least one result and a
// non-nil metadata.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	response := &Response{Results: slices.Clone(results), Metadata: metadata}
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
	if len(r.Results) == 0 {
		return fmt.Errorf("%w: at least one result is required", ErrInvalidResponse)
	}
	dimensions := -1
	for i, result := range r.Results {
		if err := result.Validate(); err != nil {
			return fmt.Errorf("%w: results[%d]: %w", ErrInvalidResponse, i, err)
		}
		if dimensions < 0 {
			dimensions = len(result.Embedding)
		} else if len(result.Embedding) != dimensions {
			return fmt.Errorf("%w: results[%d]: dimensions = %d, want %d", ErrInvalidResponse, i, len(result.Embedding), dimensions)
		}
	}
	if err := r.Metadata.validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

// First returns the first embedding — the common single-input shortcut.
// Returns nil when Results is empty.
func (r *Response) First() *Result {
	if r == nil || len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
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
