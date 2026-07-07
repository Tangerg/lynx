package runtime

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"
)

func TestCatalogPricingUsesProviderScopedCatalog(t *testing.T) {
	usage := &chat.Usage{PromptTokens: 1000, CompletionTokens: 250}
	info, ok := catalog.Lookup("anthropic", "claude-3-5-haiku-20241022")
	if !ok {
		t.Fatal("test fixture model missing from catalog")
	}

	got := CatalogPricing()("anthropic", "claude-3-5-haiku-20241022", usage)
	want := chat.CostOf(info.Pricing, usage)
	if got != want {
		t.Fatalf("CatalogPricing = %v, want %v", got, want)
	}
}

func TestCatalogPricingUnknownProviderIsZero(t *testing.T) {
	got := CatalogPricing()("does-not-exist", "claude-3-5-haiku-20241022", &chat.Usage{PromptTokens: 1000})
	if got != 0 {
		t.Fatalf("CatalogPricing for unknown provider = %v, want 0", got)
	}
}
