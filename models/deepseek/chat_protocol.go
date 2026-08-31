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

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("deepseek: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("deepseek: DefaultOptions: %w", err)
	}
	if err := (RequestOptions{}).ValidateFor(c.DefaultOptions, nil, false); err != nil {
		return fmt.Errorf("deepseek: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements DeepSeek's chat protocol.
type Chat = openai.ChatCompletions

func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	dialect := openai.ReasoningContentToolReplayDialect("deepseek")
	dialect.NativeOutputFormat = func(formatType corechat.OutputFormatType) bool {
		return formatType == corechat.OutputFormatText || formatType == corechat.OutputFormatJSON
	}
	dialect.PrepareRequest = requestDialect{defaults: config.DefaultOptions.Clone()}.prepareRequest
	dialect.DisableRawRequestExtension = true
	protocol, err := openai.NewCompatibleChatCompletions(
		openai.ChatCompletionsConfig{
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
