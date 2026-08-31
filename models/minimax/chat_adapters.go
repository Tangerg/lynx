package minimax

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
	OpenAIRequestExtensionKey        = "minimax/openai_request"
	OpenAIResponseExtensionKey       = "minimax/openai_response"
	OpenAIStreamChunkExtensionKey    = "minimax/openai_stream_chunk"
	AnthropicRequestExtensionKey     = "minimax/anthropic_request"
	AnthropicResponseExtensionKey    = "minimax/anthropic_response"
	AnthropicStreamEventExtensionKey = "minimax/anthropic_stream_event"
)

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
	_ corechat.Model    = (*Messages)(nil)
	_ corechat.Streamer = (*Messages)(nil)
)

// Chat implements MiniMax's OpenAI-compatible endpoint.
type Chat = openai.ChatCompletions

// Messages implements MiniMax's Anthropic-compatible endpoint.
type Messages = anthropic.Messages

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("minimax: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("minimax: DefaultOptions: %w", err)
	}
	return nil
}

func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	reasoningDialect, err := openai.ReasoningDetailsDialect(openai.ReasoningDetailsConfig{
		Provider:     "minimax",
		TextField:    "reasoning_content",
		DetailsField: "reasoning_details",
	})
	if err != nil {
		return nil, fmt.Errorf("minimax: configure reasoning dialect: %w", err)
	}
	reasoningDialect.PrepareRequest = prepareOpenAIRequest
	reasoningDialect.TokenLimitField = openai.TokenLimitMaxCompletionTokens
	protocol, err := openai.NewCompatibleChatCompletions(
		openai.ChatCompletionsConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLIntl), HTTPClient: config.HTTPClient},
		reasoningDialect,
	)
	if err != nil {
		return nil, fmt.Errorf("minimax: construct OpenAI-compatible chat: %w", err)
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
		return errors.New("minimax: APIKey is required")
	}
	if err := m.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("minimax: DefaultOptions: %w", err)
	}
	return nil
}

func NewMessages(config MessagesConfig) (*Messages, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := anthropic.NewCompatibleMessages(anthropic.MessagesConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLIntlAnthropic), HTTPClient: config.HTTPClient}, anthropic.Dialect{Provider: "minimax"})
	if err != nil {
		return nil, fmt.Errorf("minimax: construct Anthropic-compatible chat: %w", err)
	}
	return protocol, nil
}
