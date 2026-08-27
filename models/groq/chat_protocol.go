package groq

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/openai"
)

const (
	OpenAIRequestExtensionKey     = "groq/openai_request"
	OpenAIResponseExtensionKey    = "groq/openai_response"
	OpenAIStreamChunkExtensionKey = "groq/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
)

// OpenAIChat implements Groq's OpenAI-compatible endpoint.
type OpenAIChat = openai.Chat

// OpenAIChatConfig configures Groq's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (o OpenAIChatConfig) Validate() error {
	if o.APIKey == "" {
		return errors.New("groq: APIKey is required")
	}
	if err := o.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("groq: DefaultOptions: %w", err)
	}
	return nil
}

// NewOpenAIChat constructs a Core chat adapter for Groq.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient}, openai.ReasoningDialect("groq"))
	if err != nil {
		return nil, fmt.Errorf("groq: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}
