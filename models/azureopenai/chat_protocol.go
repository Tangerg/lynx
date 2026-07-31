package azureopenai

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/openai"
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
	Endpoint       string
	APIVersion     string
	DefaultOptions corechat.Options
	RequestOptions []option.RequestOption
}

// NewChat constructs a Core chat adapter for Azure OpenAI. APIKey may be
// empty when RequestOptions provide Azure AD authentication.
func NewChat(config ChatConfig) (*Chat, error) {
	if config.Endpoint == "" {
		return nil, errors.New("azureopenai: Endpoint is required")
	}
	apiKey, requestOptions := buildAzureRequestOptions(config.APIKey, config.Endpoint, config.APIVersion, config.RequestOptions)
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: apiKey, DefaultOptions: config.DefaultOptions, RequestOptions: requestOptions}, openai.ReasoningContentDialect())
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
