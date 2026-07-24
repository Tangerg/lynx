package server

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// TestModelToWire pins the application-model → wire capability mapping (models.list): the
// full set a model picker renders — reasoning support + effort levels, the
// input/output modalities, structured output, cache pricing, and the
// identity/limit metadata — all flow through.
func TestModelToWire(t *testing.T) {
	info := models.Model{
		ID:       "claude-x",
		Provider: "anthropic",
		Details: &models.ModelDetails{
			DisplayName:      "Claude X",
			Deprecated:       true,
			KnowledgeCutoff:  time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Reasoning:        true,
			ReasoningLevels:  []string{"low", "high"},
			ReasoningDefault: "low",
			Multimodal:       true,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"text"},
			ToolUse:          true,
			StructuredOutput: true,
			ContextWindow:    200000,
			MaxInputTokens:   190000,
			MaxOutputTokens:  8192,
			Pricing:          &models.Pricing{InputPerMillion: 3, OutputPerMillion: 15, CacheReadPerMillion: 0.3, CacheWritePerMillion: 3.75},
		},
	}

	m := modelToWire(info)

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
	m := modelToWire(models.Model{
		ID:       "tiny",
		Provider: "openai",
		Details:  &models.ModelDetails{InputModalities: []string{"text"}},
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
	tests := []struct {
		id   string
		want bool
	}{
		{id: "openai-compatible", want: true},
		{id: "azureopenai", want: true},
		{id: "anthropic", want: false},
	}
	for _, test := range tests {
		wire, err := providerToWire(models.ProviderInfo{ID: test.id, RequiresBaseURL: test.want})
		if err != nil {
			t.Fatalf("providerToWire(%q): %v", test.id, err)
		}
		if wire.RequiresBaseURL != test.want {
			t.Errorf("providerToWire(%q).RequiresBaseURL = %v, want %v", test.id, wire.RequiresBaseURL, test.want)
		}
	}
}
