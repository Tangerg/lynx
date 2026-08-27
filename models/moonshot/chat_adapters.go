package moonshot

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
	OpenAIRequestExtensionKey        = "moonshot/openai_request"
	OpenAIResponseExtensionKey       = "moonshot/openai_response"
	OpenAIStreamChunkExtensionKey    = "moonshot/openai_stream_chunk"
	AnthropicRequestExtensionKey     = "moonshot/anthropic_request"
	AnthropicResponseExtensionKey    = "moonshot/anthropic_response"
	AnthropicStreamEventExtensionKey = "moonshot/anthropic_stream_event"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
	_ corechat.Model    = (*AnthropicChat)(nil)
	_ corechat.Streamer = (*AnthropicChat)(nil)
)

// OpenAIChat implements Moonshot's OpenAI-compatible endpoint.
type OpenAIChat = openai.Chat

// AnthropicChat implements Moonshot's Anthropic-compatible endpoint.
type AnthropicChat = anthropic.Chat

// OpenAIChatConfig configures Moonshot's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (o OpenAIChatConfig) Validate() error {
	if o.APIKey == "" {
		return errors.New("moonshot: APIKey is required")
	}
	if err := o.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("moonshot: DefaultOptions: %w", err)
	}
	return nil
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for Moonshot.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	dialect := openai.ReasoningContentReplayDialect("moonshot")
	dialect.PrepareRequest = prepareOpenAIRequest
	dialect.TokenLimitField = openai.TokenLimitMaxCompletionTokens
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient}, dialect)
	if err != nil {
		return nil, fmt.Errorf("moonshot: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}

// AnthropicChatConfig configures Moonshot's Anthropic-compatible Core chat adapter.
type AnthropicChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (a AnthropicChatConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("moonshot: APIKey is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("moonshot: DefaultOptions: %w", err)
	}
	return nil
}

// NewAnthropicChat constructs an Anthropic-wire Core chat adapter for Moonshot.
func NewAnthropicChat(config AnthropicChatConfig) (*AnthropicChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := anthropic.NewCompatibleChat(anthropic.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLAnthropic), HTTPClient: config.HTTPClient}, anthropic.Dialect{Provider: "moonshot"})
	if err != nil {
		return nil, fmt.Errorf("moonshot: construct Anthropic-compatible chat: %w", err)
	}
	return protocol, nil
}
