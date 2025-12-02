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
	// Sexual detects content meant to arouse sexual excitement, such as the description of sexual activity,
	// or that promotes sexual services (excluding sex education and wellness)
	Sexual Category `json:"sexual"`

	// Hate detects content that expresses, incites, or promotes hate based on race, gender, ethnicity, religion,
	// nationality, sexual orientation, disability status, or caste
	Hate Category `json:"hate"`

	// Harassment detects content that expresses, incites, or promotes harassing language towards any target
	Harassment Category `json:"harassment"`

	// SelfHarm detects content that promotes, encourages, or depicts acts of self-harm, such as suicide, cutting,
	// and eating disorders
	SelfHarm Category `json:"self_harm"`

	// SexualMinors detects sexual content that includes an individual who is under 18 years old
	SexualMinors Category `json:"sexual_minors"`

	// HateThreatening detects hateful content that also includes violence or serious harm towards the targeted
	// group based on race, gender, ethnicity, religion, nationality, sexual orientation, disability status, or caste
	HateThreatening Category `json:"hate_threatening"`

	// ViolenceGraphic detects content that depicts death, violence, or physical injury in graphic detail
	ViolenceGraphic Category `json:"violence_graphic"`

	// SelfHarmIntent detects content where the speaker expresses that they are engaging or intend to engage
	// in acts of self-harm, such as suicide, cutting, and eating disorders
	SelfHarmIntent Category `json:"self_harm_intent"`

	// SelfHarmInstructions detects content that encourages performing acts of self-harm, such as suicide, cutting,
	// and eating disorders, or that gives instructions or advice on how to commit such acts
	SelfHarmInstructions Category `json:"self_harm_instructions"`

	// HarassmentThreatening detects harassment content that also includes violence or serious harm towards any target
	HarassmentThreatening Category `json:"harassment_threatening"`

	// Violence detects content that depicts death, violence, or physical injury
	Violence Category `json:"violence"`

	// DangerousAndCriminalContent detects dangerous and criminal content
	DangerousAndCriminalContent Category `json:"dangerous_and_criminal_content"`

	// Health detects health-related misinformation
	Health Category `json:"health"`

	// Financial detects financial misinformation or fraud
	Financial Category `json:"financial"`

	// Law detects legal misinformation
	Law Category `json:"law"`

	// Pii detects personally identifiable information
	Pii Category `json:"pii"`

	// Illicit detects content that includes instructions or advice that facilitate the planning or execution
	// of wrongdoing, or that gives advice or instruction on how to commit illicit acts.
	// For example, "how to shoplift" would fit this category
	Illicit Category `json:"illicit"`

	// IllicitViolent detects content that includes instructions or advice that facilitate the planning or
	// execution of wrongdoing that also includes violence, or that gives advice or instruction on the
	// procurement of any weapon
	IllicitViolent Category `json:"illicit_violent"`
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
