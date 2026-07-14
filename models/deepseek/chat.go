package deepseek

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/openai"
)

type OpenAIChatModelConfig struct {
	APIKey         string
	DefaultOptions *chat.Options

	// BaseURL defaults to [BaseURL] (production). Set to [BaseURLBeta]
	// for prefix completion (FIM) and other experimental features.
	BaseURL string

	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c OpenAIChatModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("deepseek: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("deepseek: DefaultOptions is required")
	}
	return nil
}

// NewOpenAIChatModel returns an openai-backed [openai.ChatModel]
// pointed at DeepSeek. DeepSeek's /chat/completions is OpenAI-
// compatible — tool calling, streaming, response_format,
// reasoning_content all work out of the box.
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
