package together

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/openai"
)

// OpenAIChatConfig configures Together's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []option.RequestOption
}

// NewOpenAIChat constructs a Core chat adapter for Together.
func NewOpenAIChat(cfg OpenAIChatConfig) (*openai.Chat, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("together: APIKey is required")
	}
	requestOptions := append([]option.RequestOption{option.WithBaseURL(cmp.Or(cfg.BaseURL, BaseURL))}, cfg.RequestOptions...)
	return openai.NewChat(openai.ChatConfig{APIKey: cfg.APIKey, DefaultOptions: cfg.DefaultOptions, RequestOptions: requestOptions})
}
