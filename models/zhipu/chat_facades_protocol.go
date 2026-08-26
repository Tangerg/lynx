package zhipu

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
	OpenAIRequestExtensionKey        = "zhipu/openai_request"
	OpenAIResponseExtensionKey       = "zhipu/openai_response"
	OpenAIStreamChunkExtensionKey    = "zhipu/openai_stream_chunk"
	AnthropicRequestExtensionKey     = "zhipu/anthropic_request"
	AnthropicResponseExtensionKey    = "zhipu/anthropic_response"
	AnthropicStreamEventExtensionKey = "zhipu/anthropic_stream_event"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
	_ corechat.Model    = (*AnthropicChat)(nil)
	_ corechat.Streamer = (*AnthropicChat)(nil)
)

// OpenAIChat implements Zhipu's OpenAI-compatible endpoint.
type OpenAIChat = openai.Chat

// AnthropicChat implements Zhipu's Anthropic-compatible endpoint.
type AnthropicChat = anthropic.Chat

// OpenAIChatConfig configures Zhipu's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (o OpenAIChatConfig) Validate() error {
	if o.APIKey == "" {
		return errors.New("zhipu: APIKey is required")
	}
	if err := o.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("zhipu: DefaultOptions: %w", err)
	}
	return nil
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for Zhipu.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient}, openai.ReasoningContentReplayDialect("zhipu"))
	if err != nil {
		return nil, fmt.Errorf("zhipu: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}

// AnthropicChatConfig configures Zhipu's Anthropic-compatible Core chat adapter.
type AnthropicChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (a AnthropicChatConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("zhipu: APIKey is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("zhipu: DefaultOptions: %w", err)
	}
	return nil
}

// NewAnthropicChat constructs an Anthropic-wire Core chat adapter for Zhipu.
func NewAnthropicChat(config AnthropicChatConfig) (*AnthropicChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := anthropic.NewCompatibleChat(anthropic.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLAnthropic), HTTPClient: config.HTTPClient}, anthropic.Dialect{Provider: "zhipu"})
	if err != nil {
		return nil, fmt.Errorf("zhipu: construct Anthropic-compatible chat: %w", err)
	}
	return protocol, nil
}
