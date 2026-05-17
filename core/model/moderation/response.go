package moderation

import "errors"

// Category is one moderation dimension's verdict — a flagged bit plus a
// confidence score in [0, 1].
type Category struct {
	// Flagged is true when the content violates this category's policy.
	Flagged bool `json:"flagged"`

	// Score is the provider's confidence in the violation, 0–1.
	Score float64 `json:"score"`
}

// Moderation aggregates every category a content-moderation provider
// surfaces. Providers vary in which fields they populate — unflagged
// categories simply leave Flagged=false and Score=0.
//
// Field doc comments preserve OpenAI's category descriptions because
// the policy semantics are part of the API contract callers reason
// about.
type Moderation struct {
	// Sexual covers content meant to arouse sexual excitement or
	// promote sexual services (sex education / wellness excluded).
	Sexual Category `json:"sexual"`

	// Hate covers content expressing or promoting hate based on race,
	// gender, ethnicity, religion, nationality, sexual orientation,
	// disability status, or caste.
	Hate Category `json:"hate"`

	// Harassment covers content expressing, inciting, or promoting
	// harassing language toward any target.
	Harassment Category `json:"harassment"`

	// SelfHarm covers content promoting, encouraging, or depicting
	// acts of self-harm (suicide, cutting, eating disorders).
	SelfHarm Category `json:"self_harm"`

	// SexualMinors covers sexual content involving anyone under 18.
	SexualMinors Category `json:"sexual_minors"`

	// HateThreatening covers hateful content that also includes
	// violence or serious harm toward the targeted group.
	HateThreatening Category `json:"hate_threatening"`

	// ViolenceGraphic covers content depicting death, violence, or
	// physical injury in graphic detail.
	ViolenceGraphic Category `json:"violence_graphic"`

	// SelfHarmIntent covers content where the speaker expresses
	// intent to engage in self-harm.
	SelfHarmIntent Category `json:"self_harm_intent"`

	// SelfHarmInstructions covers content giving instructions or
	// advice on committing self-harm.
	SelfHarmInstructions Category `json:"self_harm_instructions"`

	// HarassmentThreatening covers harassment combined with violence
	// or threats of serious harm.
	HarassmentThreatening Category `json:"harassment_threatening"`

	// Violence covers content depicting death, violence, or physical
	// injury (without the "graphic" qualifier).
	Violence Category `json:"violence"`

	// DangerousAndCriminalContent covers dangerous or criminal content.
	DangerousAndCriminalContent Category `json:"dangerous_and_criminal_content"`

	// Health flags health-related misinformation.
	Health Category `json:"health"`

	// Financial flags financial misinformation or fraud.
	Financial Category `json:"financial"`

	// Law flags legal misinformation.
	Law Category `json:"law"`

	// Pii flags personally identifiable information.
	Pii Category `json:"pii"`

	// Illicit flags content giving instructions for committing illicit
	// acts (e.g. "how to shoplift").
	Illicit Category `json:"illicit"`

	// IllicitViolent flags illicit-act instructions that also involve
	// violence or weapons procurement.
	IllicitViolent Category `json:"illicit_violent"`
}

// Flagged reports whether any category fired. Useful when callers only
// need a yes/no decision without inspecting individual scores.
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
		m.Pii.Flagged ||
		m.Illicit.Flagged ||
		m.IllicitViolent.Flagged
}

// ResultMetadata holds per-input metadata returned by the provider.
type ResultMetadata struct {
	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra,omitzero"`
}

func (r *ResultMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (r *ResultMetadata) Get(key string) (any, bool) {
	if r.Extra == nil {
		return nil, false
	}
	value, exists := r.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (r *ResultMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Result is one input's moderation verdict plus metadata.
type Result struct {
	// Moderation holds the per-category verdict.
	Moderation *Moderation `json:"categories,omitempty"`

	// Metadata carries per-input extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// NewResult builds a [Result]. Returns an error when moderation or
// metadata is nil.
func NewResult(moderation *Moderation, metadata *ResultMetadata) (*Result, error) {
	if moderation == nil {
		return nil, errors.New("moderation.NewResult: moderation must not be nil")
	}
	if metadata == nil {
		return nil, errors.New("moderation.NewResult: metadata must not be nil")
	}
	return &Result{Moderation: moderation, Metadata: metadata}, nil
}

// ResponseMetadata holds response-level metadata for a moderation call.
type ResponseMetadata struct {
	// ID is the provider-assigned response id.
	ID string `json:"id"`

	// Model is the model name actually served.
	Model string `json:"model"`

	// Created is the provider-reported creation time, Unix seconds.
	Created int64 `json:"created"`

	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra,omitzero"`
}

func (r *ResponseMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (r *ResponseMetadata) Get(key string) (any, bool) {
	if r.Extra == nil {
		return nil, false
	}
	value, exists := r.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (r *ResponseMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
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
	if len(results) == 0 {
		return nil, errors.New("moderation.NewResponse: at least one Result is required")
	}
	if metadata == nil {
		return nil, errors.New("moderation.NewResponse: metadata must not be nil")
	}
	return &Response{Results: results, Metadata: metadata}, nil
}

// Result returns the first verdict — the common single-input shortcut.
// Returns nil when Results is empty.
func (r *Response) Result() *Result {
	if len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
}
