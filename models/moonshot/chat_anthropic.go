package moonshot

import (
	"cmp"
	"errors"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/anthropic"
)

type AnthropicChatModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *chat.Options

	// BaseURL selects the region. Defaults to [BaseURLAnthropic]
	// (domestic). Use [BaseURLIntlAnthropic] for the international
	// host.
	BaseURL string

	// RequestOptions reach the underlying anthropic-sdk-go client;
	// use [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c AnthropicChatModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("moonshot: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("moonshot: DefaultOptions is required")
	}
	return nil
}

// NewAnthropicChatModel returns an [anthropic.ChatModel] pointed at
// Moonshot's Anthropic-compatible endpoint. Lets callers using
// Anthropic SDK callers swap base URL to keep their integration
// while targeting Kimi-K2.
func NewAnthropicChatModel(cfg AnthropicChatModelConfig) (*anthropic.ChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, BaseURLAnthropic)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return anthropic.NewChatModel(anthropic.ChatModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
