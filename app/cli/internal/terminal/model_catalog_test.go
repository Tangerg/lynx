package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
)

func TestModelCatalogDocumentConsumesCompleteModelMetadata(t *testing.T) {
	t.Parallel()

	document := modelCatalogDocument([]agent.Model{{
		ID: "reasoner", Provider: "provider", DisplayName: "Reasoner", Deprecated: true,
		ContextWindow: 200_000, MaxInputTokens: 180_000, MaxOutputTokens: 20_000,
		KnowledgeCutoff: "2026-01-31",
		Capabilities: &agent.ModelCapabilities{
			Reasoning: true, ReasoningLevels: []string{"low", "high"}, ReasoningDefaultLevel: "high",
			Multimodal: true, InputModalities: []agent.ModelModality{agent.ModelModalityText, agent.ModelModalityImage},
			OutputModalities: []agent.ModelModality{agent.ModelModalityText}, ToolUse: true, StructuredOutput: true,
		},
		Pricing: &agent.ModelPricing{
			InputUSDPerMillionTokens: 0.2, OutputUSDPerMillionTokens: 0.8,
			CacheReadUSDPerMillionTokens: 0.02, CacheWriteUSDPerMillionTokens: 0.1,
		},
	}})
	if document.Title != "Models" || document.Detail != "1 available" || len(document.Sections) != 1 {
		t.Fatalf("document = %+v", document)
	}
	output := document.Sections[0].Title + "\n" + document.Sections[0].Text
	for _, want := range []string{
		"provider/reasoner · Reasoner · deprecated",
		"context 200,000 · input 180,000 · output 20,000",
		"knowledge     through 2026-01-31",
		"reasoning [low, high] default high · multimodal · tool use · structured output",
		"input         text, image",
		"output        text",
		"input $0.2/M · output $0.8/M · cache read $0.02/M · cache write $0.1/M",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("model document omitted %q:\n%s", want, output)
		}
	}
}

func TestModelsCommandOpensTheRuntimeCatalog(t *testing.T) {
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New()})
	host.Shows(t, "Ask lyra")
	host.Type("/models")
	host.Press(input.Enter)
	host.Shows(t, "Models")
	host.Shows(t, "mock/balanced · Mock Balanced")
	host.Shows(t, "reasoning [low, medium, high] default medium")
	host.Shows(t, "input         text, image")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	stop()
}
