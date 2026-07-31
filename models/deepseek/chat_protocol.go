package deepseek

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/openai"
)

// OpenAIChatConfig configures DeepSeek's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []option.RequestOption
}

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
)

// OpenAIChat implements DeepSeek's OpenAI-compatible chat protocol.
type OpenAIChat struct {
	protocol *openai.Chat
}

// NewOpenAIChat constructs a Core chat adapter for DeepSeek.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if config.APIKey == "" {
		return nil, errors.New("deepseek: APIKey is required")
	}
	requestOptions := append([]option.RequestOption{option.WithBaseURL(cmp.Or(config.BaseURL, BaseURL))}, config.RequestOptions...)
	protocol, err := openai.NewCompatibleChat(
		openai.ChatConfig{
			APIKey:         config.APIKey,
			DefaultOptions: config.DefaultOptions,
			RequestOptions: requestOptions,
		},
		openai.ReasoningContentToolReplayDialect(),
	)
	if err != nil {
		return nil, fmt.Errorf("deepseek: construct OpenAI-compatible chat: %w", err)
	}
	return &OpenAIChat{protocol: protocol}, nil
}

func (chat *OpenAIChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("deepseek: nil OpenAIChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *OpenAIChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) {
			yield(nil, errors.New("deepseek: nil OpenAIChat"))
		}
	}
	return chat.protocol.Stream(ctx, request)
}
