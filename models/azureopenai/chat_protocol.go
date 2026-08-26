package azureopenai

import (
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/protocol/openai"
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

// ChatConfig configures the Core chat adapter for Azure OpenAI.
type ChatConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions corechat.Options
	HTTPClient     *http.Client
}

func (config ChatConfig) Validate() error {
	if config.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
	}
	if _, err := normalizeBaseURL(config.BaseURL); err != nil {
		return err
	}
	if err := config.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("azureopenai: DefaultOptions: %w", err)
	}
	return nil
}

// NewChat constructs a Core chat adapter for Azure OpenAI's v1 endpoint.
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
