package perplexity

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/protocol/openai"
)

const (
	OpenAIResponseExtensionKey    = "perplexity/openai_response"
	OpenAIStreamChunkExtensionKey = "perplexity/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
)

// OpenAIChat implements Perplexity's OpenAI-compatible endpoint.
type OpenAIChat struct {
	protocol *openai.Chat
}

// OpenAIChatConfig configures Perplexity's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (config OpenAIChatConfig) Validate() error {
	if config.APIKey == "" {
		return errors.New("perplexity: APIKey is required")
	}
	if err := validateCoreOptions(config.DefaultOptions); err != nil {
		return fmt.Errorf("perplexity: DefaultOptions: %w", err)
	}
	return nil
}

// NewOpenAIChat constructs a Core chat adapter for Perplexity.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChat(
		openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient},
		openai.Dialect{
			Provider:                   "perplexity",
			PrepareRequest:             prepareRequest,
			DisableRawRequestExtension: true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("perplexity: construct OpenAI-compatible chat: %w", err)
	}
	return &OpenAIChat{protocol: protocol}, nil
}

func (chat *OpenAIChat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	if chat == nil || chat.protocol == nil {
		return nil, errors.New("perplexity: nil OpenAIChat")
	}
	return chat.protocol.Call(ctx, request)
}

func (chat *OpenAIChat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if chat == nil || chat.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("perplexity: nil OpenAIChat")) }
	}
	return chat.protocol.Stream(ctx, request)
}
