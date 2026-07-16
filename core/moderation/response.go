package moderation

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

// Verdict is one moderation dimension's outcome — a flagged bit plus a
// confidence score in [0, 1].
type Verdict struct {
	// Flagged is true when the content violates this category's policy.
	Flagged bool `json:"flagged"`

	// Score is the provider's confidence in the violation, 0–1.
	Score float64 `json:"score"`
}

// Categories is the provider-reported category set. Keys retain provider
// semantics instead of forcing every provider through one closed taxonomy.
type Categories map[string]Verdict

// Flagged reports whether any category fired. Useful when callers only
// need a yes/no decision without inspecting individual scores.
func (c Categories) Flagged() bool {
	for _, verdict := range c {
		if verdict.Flagged {
			return true
		}
	}
	return false
}

func (c Categories) validate() error {
	if len(c) == 0 {
		return fmt.Errorf("%w: categories must not be empty", ErrInvalidResponse)
	}
	for category, verdict := range c {
		if category == "" || strings.TrimSpace(category) != category {
			return fmt.Errorf("%w: invalid category %q", ErrInvalidResponse, category)
		}
		if err := verdict.validate(); err != nil {
			return fmt.Errorf("%w: category %q: %w", ErrInvalidResponse, category, err)
		}
	}
	return nil
}

func (v Verdict) validate() error {
	if math.IsNaN(v.Score) || math.IsInf(v.Score, 0) || v.Score < 0 || v.Score > 1 {
		return fmt.Errorf("%w: score must be finite and in [0, 1], got %v", ErrInvalidResponse, v.Score)
	}
	return nil
}

// ResultMetadata holds per-input metadata returned by the provider.
type ResultMetadata struct {
	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific result metadata into Extra.
func (m *ResultMetadata) Set(key string, value any) error {
	if m == nil {
		return fmt.Errorf("moderation.ResultMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("moderation.ResultMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

// Result is one input's moderation verdict plus metadata.
type Result struct {
	// Categories holds the per-category verdict.
	Categories Categories `json:"categories,omitzero"`

	// Metadata carries per-input extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// NewResult builds a [Result]. Returns an error when categories or
// metadata is nil.
func NewResult(categories Categories, metadata *ResultMetadata) (*Result, error) {
	result := &Result{Categories: maps.Clone(categories), Metadata: metadata}
	if err := result.validate(); err != nil {
		return nil, fmt.Errorf("moderation.NewResult: %w", err)
	}
	return result, nil
}

// ResponseMetadata holds response-level metadata for a moderation call.
type ResponseMetadata struct {
	// ID is the provider-assigned response id.
	ID string `json:"id"`

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
		return fmt.Errorf("moderation.ResponseMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("moderation.ResponseMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

// Response is the full moderation result: one [*Result] per input plus
// shared response metadata.
type Response struct {
	// Results holds one entry per input, in the same order.
	Results []*Result `json:"results,omitzero"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from at least one result and a
// non-nil metadata.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	response := &Response{Results: slices.Clone(results), Metadata: metadata}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("moderation.NewResponse: %w", err)
	}
	return response, nil
}

// Validate recursively verifies category verdicts and response metadata.
func (r *Response) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: nil response", ErrInvalidResponse)
	}
	if len(r.Results) == 0 {
		return fmt.Errorf("%w: at least one result is required", ErrInvalidResponse)
	}
	for i, result := range r.Results {
		if err := result.validate(); err != nil {
			return fmt.Errorf("%w: results[%d]: %w", ErrInvalidResponse, i, err)
		}
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
	if err := r.Categories.validate(); err != nil {
		return err
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
	if m.ID != "" && strings.TrimSpace(m.ID) != m.ID {
		return fmt.Errorf("%w: response metadata ID must not have surrounding whitespace", ErrInvalidResponse)
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

// First returns the first verdict — the common single-input shortcut.
// Returns nil when Results is empty.
func (r *Response) First() *Result {
	if r == nil || len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
}
