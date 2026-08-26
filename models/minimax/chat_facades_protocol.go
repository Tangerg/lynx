package minimax

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/protocol/anthropic"
	"github.com/Tangerg/lynx/models/protocol/openai"
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
type OpenAIChat struct {
	protocol *openai.Chat
}

// AnthropicChat implements MiniMax's Anthropic-compatible endpoint.
type AnthropicChat struct {
	protocol *anthropic.Chat
}

// OpenAIChatConfig configures MiniMax's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (config OpenAIChatConfig) Validate() error {
	if config.APIKey == "" {
		return errors.New("minimax: APIKey is required")
	}
	if err := config.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("minimax: DefaultOptions: %w", err)
	}
	return nil
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for MiniMax.
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
	return &OpenAIChat{protocol: protocol}, nil
}

// AnthropicChatConfig configures MiniMax's Anthropic-compatible Core chat adapter.
type AnthropicChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (config AnthropicChatConfig) Validate() error {
	if config.APIKey == "" {
		return errors.New("minimax: APIKey is required")
	}
	if err := config.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("minimax: DefaultOptions: %w", err)
	}
	return nil
}

// NewAnthropicChat constructs an Anthropic-wire Core chat adapter for MiniMax.
func NewAnthropicChat(config AnthropicChatConfig) (*AnthropicChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := anthropic.NewCompatibleChat(anthropic.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLIntlAnthropic), HTTPClient: config.HTTPClient}, anthropic.Dialect{Provider: "minimax"})
	if err != nil {
		return nil, fmt.Errorf("minimax: construct Anthropic-compatible chat: %w", err)
	}
	return &AnthropicChat{protocol: protocol}, nil
}

func (chat *OpenAIChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("minimax: nil OpenAIChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *OpenAIChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("minimax: nil OpenAIChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}

func (chat *AnthropicChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("minimax: nil AnthropicChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *AnthropicChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("minimax: nil AnthropicChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}
