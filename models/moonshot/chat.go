package moonshot

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/openai"
)

type OpenAIChatModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *chat.Options

	// BaseURL selects the region. Defaults to [BaseURL] (domestic).
	// Use [BaseURLIntl] for the international api.moonshot.ai host.
	BaseURL string

	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c *OpenAIChatModelConfig) validate() error {
	if c == nil {
		return errors.New("moonshot: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("moonshot: ApiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("moonshot: DefaultOptions is required")
	}
	return nil
}

// NewOpenAIChatModel returns an openai-backed chat model pointed at
// Moonshot's OpenAI-compatible /chat/completions endpoint. Tool
// calling, streaming, response_format and reasoning_content
// (K2-thinking series) all behave OpenAI-compatibly. For the
// Anthropic-shaped /v1/messages endpoint use [NewAnthropicChatModel]
// instead.
func NewOpenAIChatModel(cfg *OpenAIChatModelConfig) (*openai.ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, BaseURL)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return openai.NewChatModel(&openai.ChatModelConfig{
		ApiKey:         cfg.ApiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
