package perplexity

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/protocol/openai"
)

const (
	OpenAIResponseExtensionKey    = "perplexity/openai_response"
	OpenAIStreamChunkExtensionKey = "perplexity/openai_stream_chunk"
)

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Perplexity's chat endpoint.
type Chat = openai.ChatCompletions

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("perplexity: APIKey is required")
	}
	if err := validateCoreOptions(c.DefaultOptions); err != nil {
		return fmt.Errorf("perplexity: DefaultOptions: %w", err)
	}
	return nil
}

func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChatCompletions(
		openai.ChatCompletionsConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient},
		openai.Dialect{
			Provider:                   "perplexity",
			TokenLimitField:            openai.TokenLimitMaxTokens,
			PrepareRequest:             prepareRequest,
			DisableRawRequestExtension: true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("perplexity: construct OpenAI-compatible chat: %w", err)
	}
	return protocol, nil
}
