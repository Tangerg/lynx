package moderation

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/scope/core/metadata"
)

// Verdict is one moderation dimension's outcome — a flagged bit plus a
// confidence score in [0, 1].
type Verdict struct {
	// Flagged is true when the content violates this category's policy.
	Flagged bool `json:"flagged"`

	// Score is the provider's confidence in the violation, 0–1.
	Score float64 `json:"score"`
}

func (v Verdict) validate() error {
	if math.IsNaN(v.Score) || math.IsInf(v.Score, 0) || v.Score < 0 || v.Score > 1 {
		return fmt.Errorf("%w: score must be finite and in [0, 1], got %v", ErrInvalidResponse, v.Score)
	}
	return nil
}

func (v Verdict) MarshalJSON() ([]byte, error) {
	if err := v.validate(); err != nil {
		return nil, err
	}
	type wireVerdict Verdict
	return json.Marshal(wireVerdict(v))
}

func (v *Verdict) UnmarshalJSON(data []byte) error {
	if v == nil {
		return fmt.Errorf("%w: verdict receiver is nil", ErrInvalidResponse)
	}
	type wireVerdict Verdict
	var decoded wireVerdict
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode verdict: %w", ErrInvalidResponse, err)
	}
	candidate := Verdict(decoded)
	if err := candidate.validate(); err != nil {
		return err
	}
	*v = candidate
	return nil
}

// Categories is the provider-reported category set. Keys retain provider
// semantics instead of forcing every provider through one closed taxonomy;
// Flagged collapses the open set only when a caller needs a yes/no policy gate.
type Categories map[string]Verdict

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

func (c Categories) MarshalJSON() ([]byte, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	type wireCategories Categories
	return json.Marshal(wireCategories(c))
}

func (c *Categories) UnmarshalJSON(data []byte) error {
	if c == nil {
		return fmt.Errorf("%w: categories receiver is nil", ErrInvalidResponse)
	}
	type wireCategories Categories
	var decoded wireCategories
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode categories: %w", ErrInvalidResponse, err)
	}
	candidate := Categories(decoded)
	if err := candidate.validate(); err != nil {
		return err
	}
	*c = candidate
	return nil
}

// OutputMetadata holds per-input metadata returned by the provider.
type OutputMetadata struct {
	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

func (o *OutputMetadata) Set(key string, value any) error {
	if o == nil {
		return fmt.Errorf("moderation: set output metadata: %w: nil receiver", ErrInvalidResponse)
	}
	if err := o.Extra.Set(key, value); err != nil {
		return fmt.Errorf("moderation: set output metadata: %w: %w", ErrInvalidResponse, err)
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
		return fmt.Errorf("%w: output metadata receiver is nil", ErrInvalidResponse)
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

// Output is one input's moderation verdict plus metadata.
type Output struct {
	// Categories holds the per-category verdict.
	Categories Categories `json:"categories,omitzero"`

	// Metadata carries per-input extras.
	Metadata *OutputMetadata `json:"metadata,omitempty"`
}

func NewOutput(categories Categories, metadata *OutputMetadata) (*Output, error) {
	output := &Output{Categories: maps.Clone(categories), Metadata: metadata}
	if err := output.Validate(); err != nil {
		return nil, fmt.Errorf("moderation: create output: %w", err)
	}
	return output, nil
}

func (o *Output) Validate() error {
	if o == nil {
		return fmt.Errorf("%w: output must not be nil", ErrInvalidResponse)
	}
	if err := o.Categories.validate(); err != nil {
		return err
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
		return fmt.Errorf("%w: output receiver is nil", ErrInvalidResponse)
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

func (r *ResponseMetadata) Set(key string, value any) error {
	if r == nil {
		return fmt.Errorf("moderation: set response metadata: %w: nil receiver", ErrInvalidResponse)
	}
	if err := r.Extra.Set(key, value); err != nil {
		return fmt.Errorf("moderation: set response metadata: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (r *ResponseMetadata) validate() error {
	if r == nil {
		return fmt.Errorf("%w: response metadata must not be nil", ErrInvalidResponse)
	}
	if r.ID != "" && strings.TrimSpace(r.ID) != r.ID {
		return fmt.Errorf("%w: response metadata ID must not have surrounding whitespace", ErrInvalidResponse)
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
		return fmt.Errorf("%w: response metadata receiver is nil", ErrInvalidResponse)
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

// Response is the full moderation output: one [*Output] per input plus
// shared response metadata.
type Response struct {
	// Outputs holds one entry per input, in the same order.
	Outputs []*Output `json:"outputs,omitzero"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

func NewResponse(outputs []*Output, metadata *ResponseMetadata) (*Response, error) {
	response := &Response{Outputs: slices.Clone(outputs), Metadata: metadata}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("moderation: create response: %w", err)
	}
	return response, nil
}

func (r *Response) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: nil response", ErrInvalidResponse)
	}
	if len(r.Outputs) == 0 {
		return fmt.Errorf("%w: at least one output is required", ErrInvalidResponse)
	}
	for i, output := range r.Outputs {
		if err := output.Validate(); err != nil {
			return fmt.Errorf("%w: outputs[%d]: %w", ErrInvalidResponse, i, err)
		}
	}
	if err := r.Metadata.validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

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
		return fmt.Errorf("%w: response receiver is nil", ErrInvalidResponse)
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
