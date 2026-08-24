package modelclient

import (
	"strings"
	"testing"
)

func TestEmbeddingSpaceIDIncludesEndpointIdentityWithoutPersistingIt(t *testing.T) {
	const (
		provider  = "openai-compatible"
		model     = "text-embedding"
		firstURL  = "https://first.example.test/v1"
		secondURL = "https://second.example.test/v1"
	)

	first := embeddingSpaceID(provider, model, firstURL)
	if first == embeddingSpaceID(provider, model, secondURL) {
		t.Fatal("embedding space did not change with the endpoint")
	}
	if first != embeddingSpaceID(provider, model, firstURL) {
		t.Fatal("embedding space is not deterministic")
	}
	if strings.Contains(first, firstURL) {
		t.Fatal("embedding space persisted the raw endpoint")
	}
}
