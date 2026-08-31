package ollama

import (
	"cmp"
	"fmt"
	"net/http"
	"strings"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/openai"
)

const (
	OpenAIRequestExtensionKey     = "ollama/openai_request"
	OpenAIResponseExtensionKey    = "ollama/openai_response"
	OpenAIStreamChunkExtensionKey = "ollama/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*ChatCompletions)(nil)
	_ corechat.Streamer = (*ChatCompletions)(nil)
)

// ChatCompletions implements Ollama's OpenAI-compatible endpoint.
type ChatCompletions = openai.ChatCompletions

type ChatCompletionsConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatCompletionsConfig) Validate() error {
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("ollama: DefaultOptions: %w", err)
	}
	return nil
}

func NewChatCompletions(config ChatCompletionsConfig) (*ChatCompletions, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = "ollama"
	}
	protocol, err := openai.NewCompatibleChatCompletions(openai.ChatCompletionsConfig{APIKey: apiKey, DefaultOptions: config.DefaultOptions, BaseURL: resolveOpenAIBaseURL(config.BaseURL), HTTPClient: config.HTTPClient}, openai.Dialect{Provider: "ollama", TokenLimitField: openai.TokenLimitMaxTokens})
	if err != nil {
		return nil, fmt.Errorf("ollama: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}

func resolveOpenAIBaseURL(base string) string {
	base = strings.TrimRight(cmp.Or(base, DefaultBaseURL), "/")
	if strings.HasSuffix(base, OpenAICompatPath) {
		return base
	}
	return base + OpenAICompatPath
}
