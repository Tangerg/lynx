package embedding

import (
	"errors"
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
		return errors.New("embedding.ResultMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
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
	if err := validateResult(result); err != nil {
		return nil, fmt.Errorf("embedding.NewResult: %w", err)
	}
	return result, nil
}

// Usage records the token consumption an embedding request reported back.
// Embedding is input-only — there is no completion, reasoning, or cache
// dimension — so a single count is the whole story. Providers that report
// a "total" figure map it here: for embeddings every token is input.
type Usage struct {
	// InputTokens are tokens consumed embedding the inputs.
	InputTokens int64 `json:"input_tokens"`
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
		return errors.New("embedding.ResponseMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
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
		return errors.New("embedding.Response: nil response")
	}
	if len(r.Results) == 0 {
		return errors.New("embedding.Response: at least one result is required")
	}
	dimensions := -1
	for i, result := range r.Results {
		if err := validateResult(result); err != nil {
			return fmt.Errorf("embedding.Response: results[%d]: %w", i, err)
		}
		if dimensions < 0 {
			dimensions = len(result.Embedding)
		} else if len(result.Embedding) != dimensions {
			return fmt.Errorf("embedding.Response: results[%d]: dimensions = %d, want %d", i, len(result.Embedding), dimensions)
		}
	}
	if r.Metadata == nil {
		return errors.New("embedding.Response: metadata must not be nil")
	}
	if r.Metadata.Model != "" && strings.TrimSpace(r.Metadata.Model) != r.Metadata.Model {
		return errors.New("embedding.Response: metadata model must not have surrounding whitespace")
	}
	if r.Metadata.Usage != nil && r.Metadata.Usage.InputTokens < 0 {
		return errors.New("embedding.Response: input tokens must not be negative")
	}
	if r.Metadata.Created < 0 {
		return errors.New("embedding.Response: created must not be negative")
	}
	if err := r.Metadata.Extra.Validate(); err != nil {
		return fmt.Errorf("embedding.Response: metadata: %w", err)
	}
	return nil
}

func validateResult(result *Result) error {
	if result == nil {
		return errors.New("result must not be nil")
	}
	if len(result.Embedding) == 0 {
		return errors.New("embedding vector must not be empty")
	}
	for i, value := range result.Embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("embedding[%d] must be finite", i)
		}
	}
	if result.Metadata == nil {
		return errors.New("metadata must not be nil")
	}
	if err := result.Metadata.Extra.Validate(); err != nil {
		return fmt.Errorf("metadata: %w", err)
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
