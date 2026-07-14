package catalog

import (
	"slices"
	"time"
)

// Model is one cataloged provider model and its commercial/capability data.
type Model struct {
	ID               string     `json:"id,omitempty"`
	DisplayName      string     `json:"display_name,omitempty"`
	KnowledgeCutoff  time.Time  `json:"knowledge_cutoff,omitzero"`
	ReleaseDate      time.Time  `json:"release_date,omitzero"`
	LastUpdated      time.Time  `json:"last_updated,omitzero"`
	Deprecated       bool       `json:"deprecated,omitempty"`
	Pricing          []Pricing  `json:"pricing,omitempty"`
	Reasoning        Reasoning  `json:"reasoning,omitzero"`
	Modalities       Modalities `json:"modalities,omitzero"`
	ToolCall         bool       `json:"tool_call,omitempty"`
	StructuredOutput bool       `json:"structured_output,omitempty"`
	Limits           Limits     `json:"limits,omitzero"`
}

func (m Model) IsZero() bool {
	return m.ID == "" && m.DisplayName == "" && m.KnowledgeCutoff.IsZero() &&
		m.ReleaseDate.IsZero() && m.LastUpdated.IsZero() && !m.Deprecated &&
		len(m.Pricing) == 0 && m.Reasoning.IsZero() && m.Modalities.IsZero() &&
		!m.ToolCall && !m.StructuredOutput && m.Limits.IsZero()
}

// Reasoning describes extended-thinking support.
type Reasoning struct {
	Supported    bool     `json:"supported,omitempty"`
	Levels       []string `json:"levels,omitempty"`
	DefaultLevel string   `json:"default_level,omitempty"`
}

func (r Reasoning) IsZero() bool {
	return !r.Supported && len(r.Levels) == 0 && r.DefaultLevel == ""
}

// Limits contains token limits. Zero means unknown, not unlimited.
type Limits struct {
	ContextWindow   int64 `json:"context_window,omitempty"`
	MaxInputTokens  int64 `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int64 `json:"max_output_tokens,omitempty"`
}

func (l Limits) IsZero() bool { return l == Limits{} }

// Modality identifies a model input or output media type.
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
	ModalityPDF   Modality = "pdf"
)

// Modalities lists accepted input and emitted output media types.
type Modalities struct {
	Input  []Modality `json:"input,omitempty"`
	Output []Modality `json:"output,omitempty"`
}

func (m Modalities) IsZero() bool {
	return len(m.Input) == 0 && len(m.Output) == 0
}

func (m Modalities) AcceptsInput(modality Modality) bool {
	return slices.Contains(m.Input, modality)
}

func (m Modalities) EmitsOutput(modality Modality) bool {
	return slices.Contains(m.Output, modality)
}
