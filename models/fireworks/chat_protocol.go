package fireworks

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/openai"
)

const (
	OpenAIRequestExtensionKey     = "fireworks/openai_request"
	OpenAIResponseExtensionKey    = "fireworks/openai_response"
	OpenAIStreamChunkExtensionKey = "fireworks/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Fireworks' chat endpoint.
type Chat = openai.ChatCompletions

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("fireworks: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("fireworks: DefaultOptions: %w", err)
	}
	return nil
}

func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChatCompletions(openai.ChatCompletionsConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient}, openai.ReasoningContentReplayDialect("fireworks"))
	if err != nil {
		return nil, fmt.Errorf("fireworks: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}
