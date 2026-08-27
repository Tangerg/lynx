package azureopenai

import (
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/openai"
)

const (
	OpenAIRequestExtensionKey     = "azureopenai/openai_request"
	OpenAIResponseExtensionKey    = "azureopenai/openai_response"
	OpenAIStreamChunkExtensionKey = "azureopenai/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Azure OpenAI's Chat Completions protocol.
type Chat = openai.Chat

type ChatConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions corechat.Options
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
	}
	if _, err := normalizeBaseURL(c.BaseURL); err != nil {
		return err
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("azureopenai: DefaultOptions: %w", err)
	}
	return nil
}

func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := normalizeBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChat(openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: baseURL, HTTPClient: config.HTTPClient}, openai.Dialect{Provider: "azureopenai", TokenLimitField: openai.TokenLimitMaxTokens})
	if err != nil {
		return nil, fmt.Errorf("azureopenai: construct chat: %w", err)
	}
	return protocol, nil
}
