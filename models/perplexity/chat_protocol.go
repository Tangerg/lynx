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
	_ corechat.Model    = (*OpenAIChat)(nil)
	_ corechat.Streamer = (*OpenAIChat)(nil)
)

// OpenAIChat implements Perplexity's OpenAI-compatible endpoint.
type OpenAIChat = openai.Chat

type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (o OpenAIChatConfig) Validate() error {
	if o.APIKey == "" {
		return errors.New("perplexity: APIKey is required")
	}
	if err := validateCoreOptions(o.DefaultOptions); err != nil {
		return fmt.Errorf("perplexity: DefaultOptions: %w", err)
	}
	return nil
}

func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	protocol, err := openai.NewCompatibleChat(
		openai.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURL), HTTPClient: config.HTTPClient},
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
