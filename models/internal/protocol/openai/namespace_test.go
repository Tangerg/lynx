package openai

import "testing"

func TestModalityExtensionKeysUseEndpointNamespace(t *testing.T) {
	t.Parallel()

	if got, want := protocolModalityRequestExtensionKey("azureopenai", "embedding"), "azureopenai/embedding_request"; got != want {
		t.Fatalf("extension key = %q, want %q", got, want)
	}
}
