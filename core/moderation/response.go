package moderation

import (
	"errors"
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
		return errors.New("categories must not be empty")
	}
	for category, verdict := range c {
		if category == "" || strings.TrimSpace(category) != category {
			return fmt.Errorf("invalid category %q", category)
		}
		if math.IsNaN(verdict.Score) || math.IsInf(verdict.Score, 0) || verdict.Score < 0 || verdict.Score > 1 {
			return fmt.Errorf("category %q score must be finite and in [0, 1], got %v", category, verdict.Score)
		}
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
		return errors.New("moderation.ResultMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
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
	if err := categories.validate(); err != nil {
		return nil, fmt.Errorf("moderation.NewResult: %w", err)
	}
	if metadata == nil {
		return nil, errors.New("moderation.NewResult: metadata must not be nil")
	}
	if err := metadata.Extra.Validate(); err != nil {
		return nil, fmt.Errorf("moderation.NewResult: metadata: %w", err)
	}
	return &Result{Categories: maps.Clone(categories), Metadata: metadata}, nil
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
		return errors.New("moderation.ResponseMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
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
		return errors.New("moderation.Response: nil response")
	}
	if len(r.Results) == 0 {
		return errors.New("moderation.Response: at least one result is required")
	}
	for i, result := range r.Results {
		if err := validateResult(result); err != nil {
			return fmt.Errorf("moderation.Response: results[%d]: %w", i, err)
		}
	}
	if r.Metadata == nil {
		return errors.New("moderation.Response: metadata must not be nil")
	}
	if r.Metadata.ID != "" && strings.TrimSpace(r.Metadata.ID) != r.Metadata.ID {
		return errors.New("moderation.Response: metadata ID must not have surrounding whitespace")
	}
	if r.Metadata.Model != "" && strings.TrimSpace(r.Metadata.Model) != r.Metadata.Model {
		return errors.New("moderation.Response: metadata model must not have surrounding whitespace")
	}
	if r.Metadata.Created < 0 {
		return errors.New("moderation.Response: created must not be negative")
	}
	if err := r.Metadata.Extra.Validate(); err != nil {
		return fmt.Errorf("moderation.Response: metadata: %w", err)
	}
	return nil
}

func validateResult(result *Result) error {
	if result == nil {
		return errors.New("result must not be nil")
	}
	if err := result.Categories.validate(); err != nil {
		return err
	}
	if result.Metadata == nil {
		return errors.New("metadata must not be nil")
	}
	if err := result.Metadata.Extra.Validate(); err != nil {
		return fmt.Errorf("metadata: %w", err)
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
