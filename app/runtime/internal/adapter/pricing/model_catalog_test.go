package pricing

import (
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/catalog"
)

func TestFromModelCatalogUsesProviderAndServedModel(t *testing.T) {
	const model = "claude-opus-5"
	usage := &chat.Usage{InputTokens: 1000, OutputTokens: 250}
	info, ok := catalog.Lookup("anthropic", model)
	if !ok {
		t.Fatal("test fixture model missing from catalog")
	}

	got := FromModelCatalog()("anthropic", model, usage)
	want := catalog.CostOf(info.Pricing, catalog.Usage{InputTokens: 1000, OutputTokens: 250})
	if got != want {
		t.Fatalf("FromModelCatalog = %v, want %v", got, want)
	}
}

func TestFromModelCatalogReturnsZeroForUnknownProvider(t *testing.T) {
	got := FromModelCatalog()("does-not-exist", "claude-opus-5", &chat.Usage{InputTokens: 1000})
	if got != 0 {
		t.Fatalf("FromModelCatalog for unknown provider = %v, want 0", got)
	}
}
