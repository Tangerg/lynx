package alibaba

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

const (
	OpenAIRequestExtensionKey     = "alibaba/openai_request"
	OpenAIResponseExtensionKey    = "alibaba/openai_response"
	OpenAIStreamChunkExtensionKey = "alibaba/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
)

// OpenAIChat implements DashScope's OpenAI-compatible endpoint.
type OpenAIChat struct {
	protocol *openai.Chat
}

// OpenAIChatConfig configures the Core chat adapter for DashScope's
// OpenAI-compatible endpoint.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []option.RequestOption
}

// NewOpenAIChat constructs a Core chat adapter for DashScope.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if config.APIKey == "" {
		return nil, errors.New("alibaba: APIKey is required")
	}
	requestOptions := append([]option.RequestOption{option.WithBaseURL(cmp.Or(config.BaseURL, BaseURLChina))}, config.RequestOptions...)
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, RequestOptions: requestOptions}, openai.ReasoningContentReplayDialect("alibaba"))
	if err != nil {
		return nil, fmt.Errorf("alibaba: construct OpenAI-compatible chat: %w", err)
	}
	return &OpenAIChat{protocol: protocol}, nil
}

func (chat *OpenAIChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("alibaba: nil OpenAIChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *OpenAIChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("alibaba: nil OpenAIChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}
