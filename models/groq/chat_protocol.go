package groq

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/openai"
)

// Namespacing preserves provider-specific data without promoting it into the
// shared Core protocol or colliding with another provider.
const (
	OpenAIRequestExtensionKey     = "groq/openai_request"
	OpenAIResponseExtensionKey    = "groq/openai_response"
	OpenAIStreamChunkExtensionKey = "groq/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Groq's chat endpoint.
type Chat = openai.ChatCompletions

// ChatConfig binds provider access and defaults shared by every chat call.
type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("groq: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("groq: DefaultOptions: %w", err)
	}
	return nil
}

// NewChat rejects an invalid provider binding before the first chat call.
func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChatCompletions(openai.ChatCompletionsConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient}, openai.ReasoningDialect("groq"))
	if err != nil {
		return nil, fmt.Errorf("groq: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}
