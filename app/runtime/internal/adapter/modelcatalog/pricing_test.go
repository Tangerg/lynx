package modelcatalog

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
)

func TestPricingUsesProviderAndServedModel(t *testing.T) {
	const model = "claude-opus-5"
	usage := &chat.Usage{InputTokens: 1000, OutputTokens: 250}
	info, ok := catalog.Default.Lookup("anthropic", model)
	if !ok {
		t.Fatal("test fixture model missing from catalog")
	}

	got := Pricing()("anthropic", model, usage)
	want := info.Pricing.Cost(catalog.Usage{InputTokens: 1000, OutputTokens: 250})
	if got != want {
		t.Fatalf("Pricing = %v, want %v", got, want)
	}
}

func TestPricingReturnsZeroForUnknownProvider(t *testing.T) {
	got := Pricing()("does-not-exist", "claude-opus-5", &chat.Usage{InputTokens: 1000})
	if got != 0 {
		t.Fatalf("Pricing for unknown provider = %v, want 0", got)
	}
}
