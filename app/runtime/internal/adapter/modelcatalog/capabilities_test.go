package modelcatalog

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

func TestCatalogContainsProviderDefaults(t *testing.T) {
	for _, provider := range llm.SupportedProviders() {
		model := provider.DefaultModel()
		if model == "" {
			continue
		}
		if _, ok := catalog.Default.Lookup(string(provider), model); !ok {
			t.Errorf("catalog has no default model %q for provider %q", model, provider)
		}
	}
}

func TestProbeUsesRemoteModelsForProviderWithoutCatalogDefault(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/models" {
			t.Errorf("request = %s %s, want GET /models", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q, want bearer key", got)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"id":"served-model"}]}`))
	}))
	t.Cleanup(server.Close)

	err := (Capabilities{}).Probe(t.Context(), provider.Provider{
		ID: "openai-compatible", APIKey: "test-key", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one endpoint-owned model probe", requests)
	}
}

func TestProbeRejectsProviderWithoutCatalogOrAdvertisedModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(server.Close)

	err := (Capabilities{}).Probe(t.Context(), provider.Provider{
		ID: "openai-compatible", APIKey: "test-key", BaseURL: server.URL,
	})
	if err == nil {
		t.Fatal("Probe accepted an endpoint that advertised no usable model")
	}
}
