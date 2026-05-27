package xiaomi

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

	// BaseURL defaults to [BaseURL]. Override only when routing
	// through a private gateway (e.g. the gifted-credit Token Plan
	// host token-plan-cn.xiaomimimo.com).
	BaseURL string

	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c OpenAIChatModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("xiaomi: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("xiaomi: DefaultOptions is required")
	}
	return nil
}

// NewOpenAIChatModel returns an openai-backed chat model pointed at
// Xiaomi MiMo's OpenAI-compatible /chat/completions endpoint. Tool
// calling, streaming, reasoning_content (thinking mode on V2-pro /
// V2.5-pro) all behave OpenAI-compatibly. For the Anthropic-shaped
// /v1/messages endpoint use [NewAnthropicChatModel] instead.
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
