package moderation

import (
	"errors"
)

// Category represents a single moderation category with its flagged status and confidence score
type Category struct {
	// Flagged indicates whether the content violates this category's policy
	Flagged bool `json:"flagged"`

	// Score represents the confidence level of the violation (typically 0.0 to 1.0)
	Score float64 `json:"score"`
}

// Moderation contains all moderation categories for content analysis
// It provides comprehensive content safety checks across multiple dimensions
type Moderation struct {
	// Sexual content detection
	Sexual Category `json:"sexual"`

	// Hate speech detection
	Hate Category `json:"hate"`

	// Harassment content detection
	Harassment Category `json:"harassment"`

	// Self-harm content detection
	SelfHarm Category `json:"self_harm"`

	// Sexual content involving minors detection
	SexualMinors Category `json:"sexual_minors"`

	// Threatening hate speech detection
	HateThreatening Category `json:"hate_threatening"`

	// Graphic violence detection
	ViolenceGraphic Category `json:"violence_graphic"`

	// Self-harm intent detection
	SelfHarmIntent Category `json:"self_harm_intent"`

	// Self-harm instruction detection
	SelfHarmInstructions Category `json:"self_harm_instructions"`

	// Threatening harassment detection
	HarassmentThreatening Category `json:"harassment_threatening"`

	// Violence detection
	Violence Category `json:"violence"`

	// Dangerous and criminal content detection
	DangerousAndCriminalContent Category `json:"dangerous_and_criminal_content"`

	// Health-related misinformation detection
	Health Category `json:"health"`

	// Financial misinformation or fraud detection
	Financial Category `json:"financial"`

	// Legal misinformation detection
	Law Category `json:"law"`

	// Personally identifiable information detection
	Pii Category `json:"pii"`
}

// Flagged returns true if any moderation category is flagged
// This is a convenience method to quickly check if content violates any policy
func (m *Moderation) Flagged() bool {
	return m.Sexual.Flagged ||
		m.Hate.Flagged ||
		m.Harassment.Flagged ||
		m.SelfHarm.Flagged ||
		m.SexualMinors.Flagged ||
		m.HateThreatening.Flagged ||
		m.ViolenceGraphic.Flagged ||
		m.SelfHarmIntent.Flagged ||
		m.SelfHarmInstructions.Flagged ||
		m.HarassmentThreatening.Flagged ||
		m.Violence.Flagged ||
		m.DangerousAndCriminalContent.Flagged ||
		m.Health.Flagged ||
		m.Financial.Flagged ||
		m.Law.Flagged ||
		m.Pii.Flagged
}

// ResultMetadata holds metadata information for a single moderation result
type ResultMetadata struct {
	// Extra holds provider-specific metadata that is not part of the standard fields
	Extra map[string]any `json:"extra"`
}

func (r *ResultMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

func (r *ResultMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	value, exists := r.Extra[key]
	return value, exists
}

func (r *ResultMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Result represents a single moderation result with its associated metadata
type Result struct {
	// Moderation contains the category-wise moderation analysis
	Moderation *Moderation `json:"categories"`

	// Metadata contains additional information about the moderation result
	Metadata *ResultMetadata `json:"metadata"`
}

// NewResult creates a new Result instance
// Both moderation and metadata are required parameters
// Returns an error if either parameter is nil
func NewResult(moderation *Moderation, metadata *ResultMetadata) (*Result, error) {
	if moderation == nil {
		return nil, errors.New("moderation cannot be nil")
	}
	if metadata == nil {
		return nil, errors.New("metadata cannot be nil")
	}
	return &Result{
		Moderation: moderation,
		Metadata:   metadata,
	}, nil
}

// ResponseMetadata holds metadata information for the entire moderation response
type ResponseMetadata struct {
	// ID is the unique identifier for this moderation request
	ID string `json:"id"`

	// Model is the name of the moderation model used
	Model string `json:"model"`

	// Created is the Unix timestamp of response creation
	Created int64 `json:"created"`

	// Extra holds provider-specific metadata that is not part of the standard fields
	Extra map[string]any `json:"extra"`
}

func (r *ResponseMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

func (r *ResponseMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	value, exists := r.Extra[key]
	return value, exists
}

func (r *ResponseMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Response represents the complete response from a moderation request
// It contains one or more moderation results and associated metadata
type Response struct {
	// Results contains all moderation results for the analyzed content
	Results []*Result `json:"results"`

	// Metadata contains information about the response itself
	Metadata *ResponseMetadata `json:"metadata"`
}

// NewResponse creates a new Response instance
// At least one result is required, and metadata must be provided
// Returns an error if results is empty or metadata is nil
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("at least one result is required")
	}
	if metadata == nil {
		return nil, errors.New("metadata cannot be nil")
	}
	return &Response{
		Results:  results,
		Metadata: metadata,
	}, nil
}

// Result returns the first result from the Results slice
// Returns nil if the Results slice is empty
// This is a convenience method for accessing the primary moderation result
func (r *Response) Result() *Result {
	if len(r.Results) > 0 {
		return r.Results[0]
	}
	return nil
}
