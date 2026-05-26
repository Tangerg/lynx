package zhipu

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

	// BaseURL defaults to [BaseURLAnthropic]. Override only when
	// routing through a private gateway.
	BaseURL string

	// RequestOptions reach the underlying anthropic-sdk-go client;
	// use [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c *AnthropicChatModelConfig) validate() error {
	if c == nil {
		return errors.New("zhipu: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("zhipu: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("zhipu: DefaultOptions is required")
	}
	return nil
}

// NewAnthropicChatModel returns an [anthropic.ChatModel] pointed at
// Zhipu's Anthropic-compatible endpoint. Wire format is the standard
// /v1/messages spec — tool calling, thinking blocks, and signature
// continuity all behave as on Anthropic's first-party API.
//
// Available models: glm-4.5, glm-4.5-air, glm-4.6. Use the model id
// constants exported from this package.
func NewAnthropicChatModel(cfg *AnthropicChatModelConfig) (*anthropic.ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, BaseURLAnthropic)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return anthropic.NewChatModel(&anthropic.ChatModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
