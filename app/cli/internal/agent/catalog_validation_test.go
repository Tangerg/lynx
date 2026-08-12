package agent

import (
	"math"
	"strings"
	"testing"
)

func TestModelOwnsAndValidatesCompleteCatalogMetadata(t *testing.T) {
	t.Parallel()

	model := Model{
		ID: "reasoner", Provider: "provider", DisplayName: "Reasoner",
		ContextWindow: 200_000, MaxInputTokens: 180_000, MaxOutputTokens: 20_000,
		KnowledgeCutoff: "2026-01-31",
		Capabilities: &ModelCapabilities{
			Reasoning: true, ReasoningLevels: []string{"low", "high"}, ReasoningDefaultLevel: "high",
			Multimodal: true, InputModalities: []ModelModality{ModelModalityText, ModelModalityImage},
			OutputModalities: []ModelModality{ModelModalityText}, ToolUse: true, StructuredOutput: true,
		},
		Pricing: &ModelPricing{InputUSDPerMillionTokens: 0.2, OutputUSDPerMillionTokens: 0.8},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := model.Clone()
	cloned.Capabilities.ReasoningLevels[0] = "mutated"
	cloned.Capabilities.InputModalities[0] = ModelModalityAudio
	cloned.Pricing.InputUSDPerMillionTokens = 99
	if model.Capabilities.ReasoningLevels[0] != "low" ||
		model.Capabilities.InputModalities[0] != ModelModalityText ||
		model.Pricing.InputUSDPerMillionTokens != 0.2 {
		t.Fatal("model clone shares mutable catalog metadata")
	}
}

func TestModelRejectsContradictoryCatalogMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model Model
		want  string
	}{
		{
			name: "cutoff", want: "knowledge cutoff",
			model: Model{ID: "m", Provider: "p", KnowledgeCutoff: "yesterday"},
		},
		{
			name: "default reasoning", want: "not offered",
			model: Model{ID: "m", Provider: "p", Capabilities: &ModelCapabilities{
				Reasoning: true, ReasoningLevels: []string{"low"}, ReasoningDefaultLevel: "high",
			}},
		},
		{
			name: "duplicate modality", want: "duplicated",
			model: Model{ID: "m", Provider: "p", Capabilities: &ModelCapabilities{
				InputModalities: []ModelModality{ModelModalityText, ModelModalityText},
			}},
		},
		{
			name: "pricing", want: "finite and non-negative",
			model: Model{ID: "m", Provider: "p", Pricing: &ModelPricing{InputUSDPerMillionTokens: math.NaN()}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.model.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want %q", err, test.want)
			}
		})
	}
}
