package zhipu

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
	OpenAIRequestExtensionKey        = "zhipu/openai_request"
	OpenAIResponseExtensionKey       = "zhipu/openai_response"
	OpenAIStreamChunkExtensionKey    = "zhipu/openai_stream_chunk"
	AnthropicRequestExtensionKey     = "zhipu/anthropic_request"
	AnthropicResponseExtensionKey    = "zhipu/anthropic_response"
	AnthropicStreamEventExtensionKey = "zhipu/anthropic_stream_event"
)

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
	_ corechat.Model    = (*Messages)(nil)
	_ corechat.Streamer = (*Messages)(nil)
)

// Chat implements Zhipu's OpenAI-compatible endpoint.
type Chat = openai.ChatCompletions

// Messages implements Zhipu's Anthropic-compatible endpoint.
type Messages = anthropic.Messages

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("zhipu: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("zhipu: DefaultOptions: %w", err)
	}
	return nil
}

func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChatCompletions(openai.ChatCompletionsConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient}, openai.ReasoningContentReplayDialect("zhipu"))
	if err != nil {
		return nil, fmt.Errorf("zhipu: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}

type MessagesConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (m MessagesConfig) Validate() error {
	if m.APIKey == "" {
		return errors.New("zhipu: APIKey is required")
	}
	if err := m.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("zhipu: DefaultOptions: %w", err)
	}
	return nil
}

func NewMessages(config MessagesConfig) (*Messages, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := anthropic.NewCompatibleMessages(anthropic.MessagesConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLAnthropic), HTTPClient: config.HTTPClient}, anthropic.Dialect{Provider: "zhipu"})
	if err != nil {
		return nil, fmt.Errorf("zhipu: construct Anthropic-compatible chat: %w", err)
	}
	return protocol, nil
}
