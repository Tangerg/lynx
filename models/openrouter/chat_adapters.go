package openrouter

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/anthropic"
	"github.com/Tangerg/scope/models/protocol/openai"
)

const (
	OpenAIRequestExtensionKey        = "openrouter/openai_request"
	OpenAIResponseExtensionKey       = "openrouter/openai_response"
	OpenAIStreamChunkExtensionKey    = "openrouter/openai_stream_chunk"
	AnthropicRequestExtensionKey     = "openrouter/anthropic_request"
	AnthropicResponseExtensionKey    = "openrouter/anthropic_response"
	AnthropicStreamEventExtensionKey = "openrouter/anthropic_stream_event"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
	_ corechat.Model    = (*AnthropicChat)(nil)
	_ corechat.Streamer = (*AnthropicChat)(nil)
)

// OpenAIChat implements OpenRouter's OpenAI-compatible endpoint.
type OpenAIChat = openai.Chat

// AnthropicChat implements OpenRouter's Anthropic-compatible endpoint.
type AnthropicChat = anthropic.Chat

// OpenAIChatConfig configures OpenRouter's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	AppURL         string
	AppTitle       string
	HTTPClient     *http.Client
}

func (o OpenAIChatConfig) Validate() error {
	if o.APIKey == "" {
		return errors.New("openrouter: APIKey is required")
	}
	if err := o.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("openrouter: DefaultOptions: %w", err)
	}
	return nil
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for OpenRouter.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	dialect, err := openai.ReasoningDetailsDialect(openai.ReasoningDetailsConfig{
		Provider:        "openrouter",
		TextField:       "reasoning",
		DetailsField:    "reasoning_details",
		ReplayPlainText: true,
	})
	if err != nil {
		return nil, fmt.Errorf("openrouter: configure reasoning dialect: %w", err)
	}
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{
		APIKey: config.APIKey, DefaultOptions: config.DefaultOptions,
		BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient,
		Headers: providerHeaders(config.AppURL, config.AppTitle),
	}, dialect)
	if err != nil {
		return nil, fmt.Errorf("openrouter: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}

// AnthropicChatConfig configures OpenRouter's Anthropic-compatible Core chat adapter.
type AnthropicChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	AppURL         string
	AppTitle       string
	HTTPClient     *http.Client
}

func (a AnthropicChatConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("openrouter: APIKey is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("openrouter: DefaultOptions: %w", err)
	}
	return nil
}

// NewAnthropicChat constructs an Anthropic-wire Core chat adapter for OpenRouter.
func NewAnthropicChat(config AnthropicChatConfig) (*AnthropicChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := anthropic.NewCompatibleChat(anthropic.ChatConfig{
		APIKey: config.APIKey, DefaultOptions: config.DefaultOptions,
		BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient,
		Headers: providerHeaders(config.AppURL, config.AppTitle),
	}, anthropic.Dialect{Provider: "openrouter"})
	if err != nil {
		return nil, fmt.Errorf("openrouter: construct Anthropic-compatible chat: %w", err)
	}
	return protocol, nil
}

func providerHeaders(appURL, appTitle string) http.Header {
	headers := make(http.Header, 2)
	if appURL != "" {
		headers.Set(HeaderReferer, appURL)
	}
	if appTitle != "" {
		headers.Set(HeaderAppTitle, appTitle)
	}
	return headers
}
