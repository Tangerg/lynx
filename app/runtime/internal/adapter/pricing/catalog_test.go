package pricing

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"
)

func TestCatalogUsesProviderScopedCatalog(t *testing.T) {
	usage := &chat.Usage{PromptTokens: 1000, CompletionTokens: 250}
	info, ok := catalog.Lookup("anthropic", "claude-3-5-haiku-20241022")
	if !ok {
		t.Fatal("test fixture model missing from catalog")
	}

	got := Catalog()("anthropic", "claude-3-5-haiku-20241022", usage)
	want := catalog.CostOf(info.Pricing, catalog.Usage{InputTokens: 1000, OutputTokens: 250})
	if got != want {
		t.Fatalf("Catalog = %v, want %v", got, want)
	}
}

func TestCatalogUnknownProviderIsZero(t *testing.T) {
	got := Catalog()("does-not-exist", "claude-3-5-haiku-20241022", &chat.Usage{PromptTokens: 1000})
	if got != 0 {
		t.Fatalf("Catalog for unknown provider = %v, want 0", got)
	}
}
