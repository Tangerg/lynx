package openrouter

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
	BaseURL        string

	// AppURL populates HTTP-Referer for OpenRouter app attribution.
	AppURL string

	// AppTitle populates X-Title for OpenRouter leaderboard ranking.
	AppTitle string

	// RequestOptions reach the underlying anthropic-sdk-go client;
	// use [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c AnthropicChatModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("openrouter: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openrouter: DefaultOptions is required")
	}
	return nil
}

// NewAnthropicChatModel returns an [anthropic.ChatModel] pointed at
// OpenRouter. OpenRouter serves Anthropic-shaped /messages alongside
// /chat/completions on the same base URL; pick this constructor when
// the calling code is written against the Anthropic SDK and you want
// OpenRouter's fallback / routing semantics underneath.
//
// Note: Anthropic-shape requests are only honored for models whose
// upstream provider supports the Messages API (Claude family,
// MiniMax-M2, GLM-4.6, Kimi-K2, etc.). For other models OpenRouter
// transparently translates Messages → Chat Completions, but unique
// fields (thinking blocks, tool_use signatures) may not round-trip.
func NewAnthropicChatModel(cfg AnthropicChatModelConfig) (*anthropic.ChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, BaseURL)
	reqOpts := []option.RequestOption{option.WithBaseURL(baseURL)}
	if cfg.AppURL != "" {
		reqOpts = append(reqOpts, option.WithHeader(HeaderReferer, cfg.AppURL))
	}
	if cfg.AppTitle != "" {
		reqOpts = append(reqOpts, option.WithHeader(HeaderAppTitle, cfg.AppTitle))
	}
	reqOpts = append(reqOpts, cfg.RequestOptions...)
	return anthropic.NewChatModel(anthropic.ChatModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
