package xiaomi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"

	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openaioption "github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/openai"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
	_ corechat.Model    = (*AnthropicChat)(nil)
	_ corechat.Streamer = (*AnthropicChat)(nil)
)

// OpenAIChat implements MiMo's OpenAI-compatible endpoint.
type OpenAIChat struct {
	protocol *openai.Chat
}

// AnthropicChat implements MiMo's Anthropic-compatible endpoint.
type AnthropicChat struct {
	protocol *anthropic.Chat
}

// OpenAIChatConfig configures MiMo's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []openaioption.RequestOption
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for MiMo.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if config.APIKey == "" {
		return nil, errors.New("xiaomi: APIKey is required")
	}
	requestOptions := append([]openaioption.RequestOption{openaioption.WithBaseURL(cmp.Or(config.BaseURL, BaseURL))}, config.RequestOptions...)
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, RequestOptions: requestOptions}, openai.ReasoningContentDialect())
	if err != nil {
		return nil, fmt.Errorf("xiaomi: construct OpenAI-compatible chat: %w", err)
	}
	return &OpenAIChat{protocol: protocol}, nil
}

// AnthropicChatConfig configures MiMo's Anthropic-compatible Core chat adapter.
type AnthropicChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []anthropicoption.RequestOption
}

// NewAnthropicChat constructs an Anthropic-wire Core chat adapter for MiMo.
func NewAnthropicChat(config AnthropicChatConfig) (*AnthropicChat, error) {
	if config.APIKey == "" {
		return nil, errors.New("xiaomi: APIKey is required")
	}
	requestOptions := append([]anthropicoption.RequestOption{anthropicoption.WithBaseURL(cmp.Or(config.BaseURL, BaseURLAnthropic))}, config.RequestOptions...)
	protocol, err := anthropic.NewCompatibleChat(anthropic.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, RequestOptions: requestOptions}, anthropic.Dialect{})
	if err != nil {
		return nil, fmt.Errorf("xiaomi: construct Anthropic-compatible chat: %w", err)
	}
	return &AnthropicChat{protocol: protocol}, nil
}

func (chat *OpenAIChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("xiaomi: nil OpenAIChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *OpenAIChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("xiaomi: nil OpenAIChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}

func (chat *AnthropicChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("xiaomi: nil AnthropicChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *AnthropicChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("xiaomi: nil AnthropicChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}
