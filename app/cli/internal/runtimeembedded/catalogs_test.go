package runtimeembedded

import (
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestModelCatalogProjectsEveryPublishedModelField(t *testing.T) {
	t.Parallel()

	capabilities := &protocol.ModelCapabilities{
		Reasoning: true, ReasoningLevels: []string{"low", "high"}, ReasoningDefaultLevel: "high",
		Multimodal: true, InputModalities: []protocol.Modality{protocol.ModalityText, protocol.ModalityImage},
		OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true, StructuredOutput: true,
	}
	pricing := &protocol.ModelPricing{
		InputUSDPerMillionTokens: 0.2, OutputUSDPerMillionTokens: 0.8,
		CacheReadUSDPerMillionTokens: 0.02, CacheWriteUSDPerMillionTokens: 0.1,
	}
	stub := modelCatalogBindingStub{
		providers: protocol.NewPage([]protocol.Provider{{ID: "provider"}}),
		models: map[string]*protocol.Page[protocol.Model]{
			"provider": protocol.NewPage([]protocol.Model{{
				ID: "reasoner", Provider: "provider", DisplayName: "Reasoner",
				ContextWindow: 200_000, MaxInputTokens: 180_000, MaxOutputTokens: 20_000,
				KnowledgeCutoff: "2026-01-31", Deprecated: true,
				Capabilities: capabilities, Pricing: pricing,
			}}),
		},
	}
	runtime := &Runtime{modelCatalog: stub, meta: requestMeta("test")}
	models, err := runtime.ListModels(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %+v", models)
	}
	model := models[0]
	wantInput := []agent.ModelModality{agent.ModelModalityText, agent.ModelModalityImage}
	if model.ID != "reasoner" || model.Provider != "provider" || model.DisplayName != "Reasoner" ||
		model.ContextWindow != 200_000 || model.MaxInputTokens != 180_000 || model.MaxOutputTokens != 20_000 ||
		model.KnowledgeCutoff != "2026-01-31" || !model.Deprecated || model.Capabilities == nil || model.Pricing == nil ||
		!model.Capabilities.Reasoning || model.Capabilities.ReasoningDefaultLevel != "high" ||
		!model.Capabilities.Multimodal || !model.Capabilities.ToolUse || !model.Capabilities.StructuredOutput ||
		!reflect.DeepEqual(model.Capabilities.InputModalities, wantInput) ||
		model.Pricing.CacheWriteUSDPerMillionTokens != 0.1 {
		t.Fatalf("projected model = %+v", model)
	}
	capabilities.ReasoningLevels[0] = "mutated"
	capabilities.InputModalities[0] = protocol.ModalityAudio
	pricing.InputUSDPerMillionTokens = 99
	if model.Capabilities.ReasoningLevels[0] != "low" ||
		model.Capabilities.InputModalities[0] != agent.ModelModalityText ||
		model.Pricing.InputUSDPerMillionTokens != 0.2 {
		t.Fatal("model projection aliases runtime-owned metadata")
	}
}
