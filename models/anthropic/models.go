package anthropic

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/lynx/core/chat"
	anthropicprotocol "github.com/Tangerg/lynx/models/protocol/anthropic"
	openaiprotocol "github.com/Tangerg/lynx/models/protocol/openai"
)

const (
	Provider      = "Anthropic"
	BaseURLOpenAI = "https://api.anthropic.com/v1"

	OpenAIRequestExtensionKey     = "anthropic/openai_request"
	OpenAIResponseExtensionKey    = "anthropic/openai_response"
	OpenAIStreamChunkExtensionKey = "anthropic/openai_stream_chunk"
)

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error { return c.protocol().Validate() }

func (c ChatConfig) protocol() anthropicprotocol.ChatConfig {
	return anthropicprotocol.ChatConfig{APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

// Chat is the Anthropic Messages protocol model.
type Chat = anthropicprotocol.Chat

func NewChat(config ChatConfig) (*Chat, error) {
	return anthropicprotocol.NewChat(config.protocol())
}

type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (config OpenAIChatConfig) Validate() error {
	if config.APIKey == "" {
		return errors.New("anthropic: APIKey is required")
	}
	if err := config.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("anthropic: DefaultOptions: %w", err)
	}
	return nil
}

// OpenAIChat is Anthropic's OpenAI-compatible protocol model.
type OpenAIChat = openaiprotocol.Chat

func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return openaiprotocol.NewCompatibleChat(
		openaiprotocol.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLOpenAI), HTTPClient: config.HTTPClient},
		openaiprotocol.Dialect{Provider: "anthropic", TokenLimitField: openaiprotocol.TokenLimitMaxTokens},
	)
}

type TextEstimatorConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

func (c TextEstimatorConfig) Validate() error { return c.protocol().Validate() }

func (c TextEstimatorConfig) protocol() anthropicprotocol.TextEstimatorConfig {
	return anthropicprotocol.TextEstimatorConfig{APIKey: c.APIKey, Model: c.Model, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

// TextEstimator is Anthropic's token-counting estimator.
type TextEstimator = anthropicprotocol.TextEstimator

func NewTextEstimator(config TextEstimatorConfig) (*TextEstimator, error) {
	return anthropicprotocol.NewTextEstimator(config.protocol())
}
