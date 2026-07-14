package anthropic

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/openai"
)

// OpenAIChatConfig configures the Core chat adapter for Anthropic's
// OpenAI-compatible endpoint.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []option.RequestOption
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for Anthropic.
func NewOpenAIChat(cfg OpenAIChatConfig) (*openai.Chat, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("anthropic: APIKey is required")
	}
	requestOptions := append([]option.RequestOption{option.WithBaseURL(cmp.Or(cfg.BaseURL, BaseURLOpenAI))}, cfg.RequestOptions...)
	return openai.NewChat(openai.ChatConfig{APIKey: cfg.APIKey, DefaultOptions: cfg.DefaultOptions, RequestOptions: requestOptions})
}
