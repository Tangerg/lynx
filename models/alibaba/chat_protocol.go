package alibaba

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/openai"
)

const (
	OpenAIRequestExtensionKey     = "alibaba/openai_request"
	OpenAIResponseExtensionKey    = "alibaba/openai_response"
	OpenAIStreamChunkExtensionKey = "alibaba/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements DashScope's chat endpoint.
type Chat = openai.ChatCompletions

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("alibaba: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("alibaba: DefaultOptions: %w", err)
	}
	return nil
}

func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChatCompletions(openai.ChatCompletionsConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLChina), HTTPClient: config.HTTPClient}, openai.ReasoningContentReplayDialect("alibaba"))
	if err != nil {
		return nil, fmt.Errorf("alibaba: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}
