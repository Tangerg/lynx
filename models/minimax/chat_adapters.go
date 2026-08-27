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
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
	_ corechat.Model    = (*AnthropicChat)(nil)
	_ corechat.Streamer = (*AnthropicChat)(nil)
)

// OpenAIChat implements MiniMax's OpenAI-compatible endpoint.
type OpenAIChat = openai.Chat

// AnthropicChat implements MiniMax's Anthropic-compatible endpoint.
type AnthropicChat = anthropic.Chat

type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (o OpenAIChatConfig) Validate() error {
	if o.APIKey == "" {
		return errors.New("minimax: APIKey is required")
	}
	if err := o.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("minimax: DefaultOptions: %w", err)
	}
	return nil
}

func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
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
	protocol, err := openai.NewCompatibleChat(
		openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLIntl), HTTPClient: config.HTTPClient},
		reasoningDialect,
	)
	if err != nil {
		return nil, fmt.Errorf("minimax: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}

type AnthropicChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (a AnthropicChatConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("minimax: APIKey is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("minimax: DefaultOptions: %w", err)
	}
	return nil
}

func NewAnthropicChat(config AnthropicChatConfig) (*AnthropicChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := anthropic.NewCompatibleChat(anthropic.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLIntlAnthropic), HTTPClient: config.HTTPClient}, anthropic.Dialect{Provider: "minimax"})
	if err != nil {
		return nil, fmt.Errorf("minimax: construct Anthropic-compatible chat: %w", err)
	}
	return protocol, nil
}
