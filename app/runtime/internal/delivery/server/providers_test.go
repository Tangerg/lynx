package server

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// TestModelToWire pins the catalog → wire capability mapping (models.list): the
// full set a model picker renders — reasoning support + effort levels, the
// input/output modalities, structured output, cache pricing, and the
// identity/limit metadata — all flow through.
func TestModelToWire(t *testing.T) {
	info := chat.ModelInfo{
		ID:              "claude-x",
		DisplayName:     "Claude X",
		Deprecated:      true,
		KnowledgeCutoff: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		Reasoning:       chat.Reasoning{Supported: true, Levels: []string{"low", "high"}, DefaultLevel: "low"},
		Modalities: chat.Modalities{
			Input:  []chat.Modality{chat.ModalityText, chat.ModalityImage},
			Output: []chat.Modality{chat.ModalityText},
		},
		ToolCall:         true,
		StructuredOutput: true,
		Limits:           chat.Limits{ContextWindow: 200000, MaxInputTokens: 190000, MaxOutputTokens: 8192},
		Pricing:          []chat.Pricing{{InputPer1M: 3, OutputPer1M: 15, CacheReadPer1M: 0.3, CacheWritePer1M: 3.75}},
	}

	m := modelToWire("anthropic", info)

	if m.ID != "claude-x" || m.Provider != "anthropic" || m.DisplayName != "Claude X" {
		t.Fatalf("identity = %+v", m)
	}
	if m.ContextWindow != 200000 || m.MaxInputTokens != 190000 || m.MaxOutputTokens != 8192 {
		t.Errorf("limits = ctx %d in %d out %d", m.ContextWindow, m.MaxInputTokens, m.MaxOutputTokens)
	}
	if !m.Deprecated || m.KnowledgeCutoff != "2025-03-01" {
		t.Errorf("deprecated=%v cutoff=%q", m.Deprecated, m.KnowledgeCutoff)
	}

	c := m.Capabilities
	if c == nil {
		t.Fatal("nil capabilities")
	}
	if !c.Reasoning || c.ReasoningDefaultLevel != "low" || len(c.ReasoningLevels) != 2 {
		t.Errorf("reasoning = %+v", c)
	}
	if !c.Multimodal { // accepts image
		t.Error("multimodal should be true (accepts image)")
	}
	if len(c.InputModalities) != 2 || c.InputModalities[0] != protocol.ModalityText || c.InputModalities[1] != protocol.ModalityImage {
		t.Errorf("inputModalities = %v", c.InputModalities)
	}
	if len(c.OutputModalities) != 1 || c.OutputModalities[0] != protocol.ModalityText {
		t.Errorf("outputModalities = %v", c.OutputModalities)
	}
	if !c.ToolUse || !c.StructuredOutput {
		t.Errorf("toolUse=%v structuredOutput=%v", c.ToolUse, c.StructuredOutput)
	}

	if m.Pricing == nil ||
		m.Pricing.InputUsdPerMillionTokens != 3 ||
		m.Pricing.OutputUsdPerMillionTokens != 15 ||
		m.Pricing.CacheReadUsdPerMillionTokens != 0.3 ||
		m.Pricing.CacheWriteUsdPerMillionTokens != 3.75 {
		t.Errorf("pricing = %+v", m.Pricing)
	}
}

// TestModelToWire_TextOnly verifies the convenience flags read false for a
// plain text model: no image → multimodal false, no reasoning levels.
func TestModelToWire_TextOnly(t *testing.T) {
	m := modelToWire("openai", chat.ModelInfo{
		ID:         "tiny",
		Modalities: chat.Modalities{Input: []chat.Modality{chat.ModalityText}},
	})
	if m.Capabilities.Multimodal {
		t.Error("text-only model must not be multimodal")
	}
	if len(m.Capabilities.ReasoningLevels) != 0 || m.Capabilities.ReasoningDefaultLevel != "" {
		t.Error("non-reasoning model must carry no levels")
	}
	if m.Pricing != nil {
		t.Error("no pricing → nil ModelPricing")
	}
}

// TestProviderToWire_RequiresBaseURL pins the flag the frontend keys its
// base-URL field off: set for the no-built-in-endpoint providers, clear for
// the named vendors.
func TestProviderToWire_RequiresBaseURL(t *testing.T) {
	if !providerToWire(provider.Metadata{ID: "openai-compatible", RequiresBaseURL: true}, provider.Provider{}).RequiresBaseURL {
		t.Error("openai-compatible must require a base URL")
	}
	if !providerToWire(provider.Metadata{ID: "azureopenai", RequiresBaseURL: true}, provider.Provider{}).RequiresBaseURL {
		t.Error("azureopenai must require a base URL")
	}
	if providerToWire(provider.Metadata{ID: "anthropic"}, provider.Provider{}).RequiresBaseURL {
		t.Error("anthropic must not require a base URL")
	}
}
