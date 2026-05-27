package zhipu

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/openai"
)

type OpenAIChatModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *chat.Options
	BaseURL        string
	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c OpenAIChatModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("zhipu: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("zhipu: DefaultOptions is required")
	}
	return nil
}

// NewOpenAIChatModel returns an openai-backed chat model pointed at
// Zhipu's BigModel v4 endpoint. Tool calling, streaming,
// response_format and reasoning_content are all OpenAI-compatible.
// For the Anthropic-shaped /v1/messages endpoint use
// [NewAnthropicChatModel] instead.
func NewOpenAIChatModel(cfg OpenAIChatModelConfig) (*openai.ChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, BaseURL)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return openai.NewChatModel(openai.ChatModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
