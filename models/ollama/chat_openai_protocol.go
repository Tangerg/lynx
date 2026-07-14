package ollama

import (
	"cmp"
	"strings"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/openai"
)

// OpenAIChatConfig configures Ollama's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []option.RequestOption
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for Ollama.
func NewOpenAIChat(cfg OpenAIChatConfig) (*openai.Chat, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = "ollama"
	}
	requestOptions := append([]option.RequestOption{option.WithBaseURL(resolveOpenAIBaseURL(cfg.BaseURL))}, cfg.RequestOptions...)
	return openai.NewChat(openai.ChatConfig{APIKey: apiKey, DefaultOptions: cfg.DefaultOptions, RequestOptions: requestOptions})
}

func resolveOpenAIBaseURL(base string) string {
	base = strings.TrimRight(cmp.Or(base, DefaultBaseURL), "/")
	if strings.HasSuffix(base, OpenAICompatPath) {
		return base
	}
	return base + OpenAICompatPath
}
