package openrouter

import (
	"cmp"
	"errors"

	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openaioption "github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/openai"
)

// OpenAIChatConfig configures OpenRouter's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	AppURL         string
	AppTitle       string
	RequestOptions []openaioption.RequestOption
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for OpenRouter.
func NewOpenAIChat(cfg OpenAIChatConfig) (*openai.Chat, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("openrouter: APIKey is required")
	}
	requestOptions := []openaioption.RequestOption{openaioption.WithBaseURL(cmp.Or(cfg.BaseURL, BaseURL))}
	if cfg.AppURL != "" {
		requestOptions = append(requestOptions, openaioption.WithHeader(HeaderReferer, cfg.AppURL))
	}
	if cfg.AppTitle != "" {
		requestOptions = append(requestOptions, openaioption.WithHeader(HeaderAppTitle, cfg.AppTitle))
	}
	requestOptions = append(requestOptions, cfg.RequestOptions...)
	return openai.NewChat(openai.ChatConfig{APIKey: cfg.APIKey, DefaultOptions: cfg.DefaultOptions, RequestOptions: requestOptions})
}

// AnthropicChatConfig configures OpenRouter's Anthropic-compatible Core chat adapter.
type AnthropicChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	AppURL         string
	AppTitle       string
	RequestOptions []anthropicoption.RequestOption
}

// NewAnthropicChat constructs an Anthropic-wire Core chat adapter for OpenRouter.
func NewAnthropicChat(cfg AnthropicChatConfig) (*anthropic.Chat, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("openrouter: APIKey is required")
	}
	requestOptions := []anthropicoption.RequestOption{anthropicoption.WithBaseURL(cmp.Or(cfg.BaseURL, BaseURL))}
	if cfg.AppURL != "" {
		requestOptions = append(requestOptions, anthropicoption.WithHeader(HeaderReferer, cfg.AppURL))
	}
	if cfg.AppTitle != "" {
		requestOptions = append(requestOptions, anthropicoption.WithHeader(HeaderAppTitle, cfg.AppTitle))
	}
	requestOptions = append(requestOptions, cfg.RequestOptions...)
	return anthropic.NewChat(anthropic.ChatConfig{APIKey: cfg.APIKey, DefaultOptions: cfg.DefaultOptions, RequestOptions: requestOptions})
}
