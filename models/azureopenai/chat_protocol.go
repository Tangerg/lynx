package azureopenai

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/internal/protocol/openai"
)

const (
	OpenAIRequestExtensionKey     = "azureopenai/openai_request"
	OpenAIResponseExtensionKey    = "azureopenai/openai_response"
	OpenAIStreamChunkExtensionKey = "azureopenai/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Azure OpenAI's Chat Completions protocol.
type Chat struct {
	protocol *openai.Chat
}

// ChatConfig configures the Core chat adapter for Azure OpenAI.
type ChatConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions corechat.Options
	RequestOptions []option.RequestOption
}

// NewChat constructs a Core chat adapter for Azure OpenAI's v1 endpoint.
func NewChat(config ChatConfig) (*Chat, error) {
	if config.APIKey == "" {
		return nil, errors.New("azureopenai: APIKey is required")
	}
	requestOptions, err := buildRequestOptions(config.BaseURL, config.RequestOptions)
	if err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, RequestOptions: requestOptions}, openai.Dialect{Provider: "azureopenai"})
	if err != nil {
		return nil, fmt.Errorf("azureopenai: construct chat: %w", err)
	}
	return &Chat{protocol: protocol}, nil
}

func (chat *Chat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("azureopenai: nil Chat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *Chat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("azureopenai: nil Chat")) }
	}
	return chat.protocol.Stream(ctx, request)
}
