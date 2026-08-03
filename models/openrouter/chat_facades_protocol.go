package openrouter

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/internal/protocol/anthropic"
	"github.com/Tangerg/lynx/models/protocol/openai"
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
type OpenAIChat struct {
	protocol *openai.Chat
}

// AnthropicChat implements OpenRouter's Anthropic-compatible endpoint.
type AnthropicChat struct {
	protocol *anthropic.Chat
}

// OpenAIChatConfig configures OpenRouter's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	AppURL         string
	AppTitle       string
	HTTPClient     *http.Client
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for OpenRouter.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if config.APIKey == "" {
		return nil, errors.New("openrouter: APIKey is required")
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
	return &OpenAIChat{protocol: protocol}, nil
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

// NewAnthropicChat constructs an Anthropic-wire Core chat adapter for OpenRouter.
func NewAnthropicChat(config AnthropicChatConfig) (*AnthropicChat, error) {
	if config.APIKey == "" {
		return nil, errors.New("openrouter: APIKey is required")
	}
	protocol, err := anthropic.NewCompatibleChat(anthropic.ChatConfig{
		APIKey: config.APIKey, DefaultOptions: config.DefaultOptions,
		BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient,
		Headers: providerHeaders(config.AppURL, config.AppTitle),
	}, anthropic.Dialect{Provider: "openrouter"})
	if err != nil {
		return nil, fmt.Errorf("openrouter: construct Anthropic-compatible chat: %w", err)
	}
	return &AnthropicChat{protocol: protocol}, nil
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

func (chat *OpenAIChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("openrouter: nil OpenAIChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *OpenAIChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("openrouter: nil OpenAIChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}

func (chat *AnthropicChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("openrouter: nil AnthropicChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *AnthropicChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) {
			yield(nil, errors.New("openrouter: nil AnthropicChat"))
		}
	}
	return chat.protocol.Stream(ctx, request)
}
