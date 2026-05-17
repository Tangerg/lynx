package anthropic

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

	// BaseURL defaults to [BaseURLOpenAI]. The anthropic-openai
	// bridge currently only ships at the bare /v1 path on
	// api.anthropic.com.
	BaseURL string

	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c *OpenAIChatModelConfig) validate() error {
	if c == nil {
		return errors.New("anthropic: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("anthropic: ApiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("anthropic: DefaultOptions is required")
	}
	return nil
}

// NewOpenAIChatModel returns an openai-backed [openai.ChatModel]
// pointed at Anthropic's first-party OpenAI-compatible endpoint. Use
// this constructor to keep an OpenAI-SDK integration intact while
// targeting Claude models; for native /v1/messages features
// (reasoning_content, computer-use blocks, citations) use
// [NewChatModel] instead.
//
// Note: the bridge is wire-format-only — Anthropic-specific request
// fields not in the OpenAI schema (cache control, computer-use,
// fine-grained tool-result blocks) are not exposed here.
func NewOpenAIChatModel(cfg *OpenAIChatModelConfig) (*openai.ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, BaseURLOpenAI)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return openai.NewChatModel(&openai.ChatModelConfig{
		ApiKey:         cfg.ApiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
