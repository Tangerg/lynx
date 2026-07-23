package models

import "time"

// Model is the application-facing catalog record used by model selection. It
// carries provider capability facts without exposing the infrastructure catalog
// or a protocol-specific shape.
type Model struct {
	ID       string
	Provider string
	Details  *ModelDetails
}

// ModelDetails is the static capability and commercial metadata known for a
// model. A nil Details means a provider endpoint reported an otherwise unknown
// model id, so callers can still select it without inventing capabilities.
type ModelDetails struct {
	DisplayName      string
	ContextWindow    int
	MaxInputTokens   int
	MaxOutputTokens  int
	KnowledgeCutoff  time.Time
	Deprecated       bool
	Reasoning        bool
	ReasoningLevels  []string
	ReasoningDefault string
	Multimodal       bool
	InputModalities  []string
	OutputModalities []string
	ToolUse          bool
	StructuredOutput bool
	Pricing          *Pricing
}

// Pricing is the primary per-million-token rate the runtime displays for a
// model. Zero-valued cache rates mean the provider does not price them
// separately.
type Pricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}
