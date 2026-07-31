package modelcatalog

import (
	"testing"

	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

func TestCatalogContainsProviderDefaults(t *testing.T) {
	for _, provider := range llm.SupportedProviders() {
		model := provider.DefaultModel()
		if model == "" {
			continue
		}
		if _, ok := catalog.Lookup(string(provider), model); !ok {
			t.Errorf("catalog has no default model %q for provider %q", model, provider)
		}
	}
}
