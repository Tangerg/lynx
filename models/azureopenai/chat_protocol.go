package azureopenai

import (
	"fmt"

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
type Chat = openai.ChatCompletions

type ChatConfig struct {
	Config
	DefaultOptions corechat.Options
}

func (c ChatConfig) resolve() (endpointConfig, error) {
	return resolveChatConfig(c.Config, c.DefaultOptions.Validate)
}

func (c ChatConfig) Validate() error {
	_, err := c.resolve()
	return err
}

func NewChat(config ChatConfig) (*Chat, error) {
	endpoint, err := config.resolve()
	if err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChatCompletions(openai.ChatCompletionsConfig{APIKey: endpoint.apiKey, DefaultOptions: config.DefaultOptions, BaseURL: endpoint.baseURL, HTTPClient: endpoint.httpClient}, openai.Dialect{Provider: protocolProvider, TokenLimitField: openai.TokenLimitMaxTokens})
	if err != nil {
		return nil, fmt.Errorf("azureopenai: construct chat: %w", err)
	}
	return protocol, nil
}
