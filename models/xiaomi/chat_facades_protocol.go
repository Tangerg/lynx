package xiaomi

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/protocol/anthropic"
	"github.com/Tangerg/lynx/models/protocol/openai"
)

const (
	OpenAIRequestExtensionKey        = "xiaomi/openai_request"
	OpenAIResponseExtensionKey       = "xiaomi/openai_response"
	OpenAIStreamChunkExtensionKey    = "xiaomi/openai_stream_chunk"
	AnthropicRequestExtensionKey     = "xiaomi/anthropic_request"
	AnthropicResponseExtensionKey    = "xiaomi/anthropic_response"
	AnthropicStreamEventExtensionKey = "xiaomi/anthropic_stream_event"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
	_ corechat.Model    = (*AnthropicChat)(nil)
	_ corechat.Streamer = (*AnthropicChat)(nil)
)

// OpenAIChat implements MiMo's OpenAI-compatible endpoint.
type OpenAIChat = openai.Chat

// AnthropicChat implements MiMo's Anthropic-compatible endpoint.
type AnthropicChat = anthropic.Chat

// OpenAIChatConfig configures MiMo's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (o OpenAIChatConfig) Validate() error {
	if o.APIKey == "" {
		return errors.New("xiaomi: APIKey is required")
	}
	if err := o.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("xiaomi: DefaultOptions: %w", err)
	}
	return nil
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for MiMo.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	dialect := openai.ReasoningContentToolReplayDialect("xiaomi")
	dialect.PrepareRequest = prepareOpenAIRequest
	dialect.TokenLimitField = openai.TokenLimitMaxCompletionTokens
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient}, dialect)
	if err != nil {
		return nil, fmt.Errorf("xiaomi: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}

// AnthropicChatConfig configures MiMo's Anthropic-compatible Core chat adapter.
type AnthropicChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (a AnthropicChatConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("xiaomi: APIKey is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("xiaomi: DefaultOptions: %w", err)
	}
	return nil
}

// NewAnthropicChat constructs an Anthropic-wire Core chat adapter for MiMo.
func NewAnthropicChat(config AnthropicChatConfig) (*AnthropicChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := anthropic.NewCompatibleChat(anthropic.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLAnthropic), HTTPClient: config.HTTPClient}, anthropic.Dialect{Provider: "xiaomi"})
	if err != nil {
		return nil, fmt.Errorf("xiaomi: construct Anthropic-compatible chat: %w", err)
	}
	return protocol, nil
}
