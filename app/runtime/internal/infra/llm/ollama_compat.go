package llm

import (
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	openaiprotocol "github.com/Tangerg/scope/models/protocol/openai"
)

const (
	ollamaProtocolProvider     = "ollama"
	ollamaCompatibilityAPIKey  = "ollama"
	defaultOllamaOpenAIBaseURL = "http://127.0.0.1:11434/v1"
)

// buildOllamaChatModel uses the daemon's supported OpenAI-compatible surface.
// Keeping the protocol adapter provider-scoped preserves ollama/* extension
// ownership without importing Ollama's server repository into the Runtime.
func buildOllamaChatModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return openaiprotocol.NewCompatibleChat(openaiprotocol.ChatConfig{
		APIKey:         ollamaAPIKey(spec.APIKey),
		DefaultOptions: opts,
		BaseURL:        ollamaOpenAIBaseURL(spec.BaseURL),
	}, openaiprotocol.Dialect{
		Provider: ollamaProtocolProvider, TokenLimitField: openaiprotocol.TokenLimitMaxTokens,
	})
}

// buildOllamaEmbeddingModel uses /v1/embeddings for the same reason as chat:
// Runtime needs a client protocol, not Ollama's model-management server module.
func buildOllamaEmbeddingModel(spec ClientSpec, opts embedding.Options) (embedding.Model, error) {
	return openaiprotocol.NewEmbeddingModel(openaiprotocol.EmbeddingModelConfig{
		Provider:       ollamaProtocolProvider,
		APIKey:         ollamaAPIKey(spec.APIKey),
		DefaultOptions: opts,
		BaseURL:        ollamaOpenAIBaseURL(spec.BaseURL),
	})
}

func ollamaAPIKey(configured string) string {
	if configured != "" {
		return configured
	}
	return ollamaCompatibilityAPIKey
}

func ollamaOpenAIBaseURL(configured string) string {
	baseURL := strings.TrimRight(configured, "/")
	if baseURL == "" {
		return defaultOllamaOpenAIBaseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL
	}
	return baseURL + "/v1"
}
