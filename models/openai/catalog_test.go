package openai_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/openai"
)

// TestNewChatModel_PricingFromCatalog proves NewChatModel fills
// Metadata().Model from the embedded catalog by the configured model id —
// both for OpenAI itself and for the OpenAI-compat delegation path, where
// a provider (deepseek, groq, …) builds an openai ChatModel with its own
// Provider override and the lookup keys off info.Provider.
func TestNewChatModel_PricingFromCatalog(t *testing.T) {
	opts, err := chat.NewOptions("gpt-4o-mini")
	if err != nil {
		t.Fatal(err)
	}
	m, err := openai.NewChatModel(openai.ChatModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Metadata().Model.Pricing[0].InputPer1M; got != 0.15 {
		t.Errorf("openai pricing input = %v, want 0.15", got)
	}

	opts2, err := chat.NewOptions("deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	m2, err := openai.NewChatModel(openai.ChatModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts2,
		Metadata:       &chat.ModelMetadata{Provider: "DeepSeek"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := m2.Metadata().Model.Pricing[0].InputPer1M; got != 0.14 {
		t.Errorf("compat (deepseek via openai) pricing input = %v, want 0.14", got)
	}
}
