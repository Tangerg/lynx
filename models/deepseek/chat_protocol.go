package deepseek

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/openai"
)

const (
	OpenAIResponseExtensionKey    = "deepseek/openai_response"
	OpenAIStreamChunkExtensionKey = "deepseek/openai_stream_chunk"
)

// OpenAIChatConfig configures DeepSeek's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (o OpenAIChatConfig) Validate() error {
	if o.APIKey == "" {
		return errors.New("deepseek: APIKey is required")
	}
	if err := o.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("deepseek: DefaultOptions: %w", err)
	}
	if err := (RequestOptions{}).ValidateFor(o.DefaultOptions, nil, false); err != nil {
		return fmt.Errorf("deepseek: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
)

// OpenAIChat implements DeepSeek's OpenAI-compatible chat protocol.
type OpenAIChat = openai.Chat

// NewOpenAIChat constructs a Core chat adapter for DeepSeek.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	dialect := openai.ReasoningContentToolReplayDialect("deepseek")
	dialect.NativeOutputFormat = func(formatType corechat.OutputFormatType) bool {
		return formatType == corechat.OutputFormatText || formatType == corechat.OutputFormatJSON
	}
	dialect.PrepareRequest = requestDialect{defaults: config.DefaultOptions.Clone()}.prepareRequest
	dialect.DisableRawRequestExtension = true
	protocol, err := openai.NewCompatibleChat(
		openai.ChatConfig{
			APIKey:         config.APIKey,
			DefaultOptions: config.DefaultOptions,
			BaseURL:        cmp.Or(config.BaseURL, BaseURL),
			HTTPClient:     config.HTTPClient,
		},
		dialect,
	)
	if err != nil {
		return nil, fmt.Errorf("deepseek: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}
