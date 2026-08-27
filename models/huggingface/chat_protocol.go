package huggingface

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/openai"
)

const (
	OpenAIRequestExtensionKey     = "huggingface/openai_request"
	OpenAIResponseExtensionKey    = "huggingface/openai_response"
	OpenAIStreamChunkExtensionKey = "huggingface/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
)

// OpenAIChat implements the Hugging Face router's OpenAI-compatible endpoint.
type OpenAIChat = openai.Chat

// OpenAIChatConfig configures Hugging Face's OpenAI-compatible Core chat adapter.
type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (o OpenAIChatConfig) Validate() error {
	if o.APIKey == "" {
		return errors.New("huggingface: APIKey is required")
	}
	if err := o.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("huggingface: DefaultOptions: %w", err)
	}
	return nil
}

// NewOpenAIChat constructs a Core chat adapter for the Hugging Face router.
func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, DefaultBaseURL), HTTPClient: config.HTTPClient}, openai.Dialect{Provider: "huggingface", TokenLimitField: openai.TokenLimitMaxTokens})
	if err != nil {
		return nil, fmt.Errorf("huggingface: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}
