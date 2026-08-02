package zhipu

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"

	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openaioption "github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/internal/protocol/anthropic"
	"github.com/Tangerg/lynx/models/internal/protocol/openai"
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
type OpenAIChat struct {
	protocol *openai.Chat
}

// AnthropicChat implements Zhipu's Anthropic-compatible endpoint.
type AnthropicChat struct {
	protocol *anthropic.Chat
}

// OpenAIChatConfig configures Zhipu's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []openaioption.RequestOption
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for Zhipu.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if config.APIKey == "" {
		return nil, errors.New("zhipu: APIKey is required")
	}
	requestOptions := append([]openaioption.RequestOption{openaioption.WithBaseURL(cmp.Or(config.BaseURL, BaseURL))}, config.RequestOptions...)
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, RequestOptions: requestOptions}, openai.ReasoningContentReplayDialect("zhipu"))
	if err != nil {
		return nil, fmt.Errorf("zhipu: construct OpenAI-compatible chat: %w", err)
	}
	return &OpenAIChat{protocol: protocol}, nil
}

// AnthropicChatConfig configures Zhipu's Anthropic-compatible Core chat adapter.
type AnthropicChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []anthropicoption.RequestOption
}

// NewAnthropicChat constructs an Anthropic-wire Core chat adapter for Zhipu.
func NewAnthropicChat(config AnthropicChatConfig) (*AnthropicChat, error) {
	if config.APIKey == "" {
		return nil, errors.New("zhipu: APIKey is required")
	}
	requestOptions := append([]anthropicoption.RequestOption{anthropicoption.WithBaseURL(cmp.Or(config.BaseURL, BaseURLAnthropic))}, config.RequestOptions...)
	protocol, err := anthropic.NewCompatibleChat(anthropic.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, RequestOptions: requestOptions}, anthropic.Dialect{Provider: "zhipu"})
	if err != nil {
		return nil, fmt.Errorf("zhipu: construct Anthropic-compatible chat: %w", err)
	}
	return &AnthropicChat{protocol: protocol}, nil
}

func (chat *OpenAIChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("zhipu: nil OpenAIChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *OpenAIChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("zhipu: nil OpenAIChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}

func (chat *AnthropicChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("zhipu: nil AnthropicChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *AnthropicChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("zhipu: nil AnthropicChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}
