package ollama

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/openai"
)

const (
	OpenAIRequestExtensionKey     = "ollama/openai_request"
	OpenAIResponseExtensionKey    = "ollama/openai_response"
	OpenAIStreamChunkExtensionKey = "ollama/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
)

// OpenAIChat implements Ollama's OpenAI-compatible endpoint.
type OpenAIChat struct {
	protocol *openai.Chat
}

// OpenAIChatConfig configures Ollama's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	RequestOptions []option.RequestOption
}

// NewOpenAIChat constructs an OpenAI-wire Core chat adapter for Ollama.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = "ollama"
	}
	requestOptions := append([]option.RequestOption{option.WithBaseURL(resolveOpenAIBaseURL(config.BaseURL))}, config.RequestOptions...)
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: apiKey, DefaultOptions: config.DefaultOptions, RequestOptions: requestOptions}, openai.Dialect{Provider: "ollama"})
	if err != nil {
		return nil, fmt.Errorf("ollama: construct OpenAI-compatible chat: %w", err)
	}
	return &OpenAIChat{protocol: protocol}, nil
}

func (chat *OpenAIChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("ollama: nil OpenAIChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *OpenAIChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("ollama: nil OpenAIChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}

func resolveOpenAIBaseURL(base string) string {
	base = strings.TrimRight(cmp.Or(base, DefaultBaseURL), "/")
	if strings.HasSuffix(base, OpenAICompatPath) {
		return base
	}
	return base + OpenAICompatPath
}
