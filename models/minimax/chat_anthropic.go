package minimax

import (
	"cmp"
	"errors"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/anthropic"
)

type AnthropicChatModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *chat.Options

	// BaseURL selects the Anthropic-compatible endpoint. Defaults to
	// [BaseURLIntlAnthropic] (USD); pass [BaseURLChinaAnthropic] for
	// RMB billing.
	BaseURL string

	// RequestOptions reach the underlying anthropic-sdk-go client;
	// use [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c *AnthropicChatModelConfig) validate() error {
	if c == nil {
		return errors.New("minimax: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("minimax: ApiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("minimax: DefaultOptions is required")
	}
	return nil
}

// NewAnthropicChatModel returns an [anthropic.ChatModel] pointed at
// MiniMax's Anthropic-compatible endpoint. The wire format is the
// /v1/messages spec — tool calling, thinking blocks, and citations
// all behave as on Anthropic's first-party API.
//
// MiniMax-M2 in particular is the headline model on this endpoint;
// other MiniMax chat models are accessible via [NewChatModel] (the
// OpenAI-compatible flavor).
func NewAnthropicChatModel(cfg *AnthropicChatModelConfig) (*anthropic.ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, BaseURLIntlAnthropic)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return anthropic.NewChatModel(&anthropic.ChatModelConfig{
		ApiKey:         cfg.ApiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
