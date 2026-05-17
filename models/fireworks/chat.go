package fireworks

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
	BaseURL        string
	RequestOptions []option.RequestOption
}

func (c *OpenAIChatModelConfig) validate() error {
	if c == nil {
		return errors.New("fireworks: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("fireworks: ApiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("fireworks: DefaultOptions is required")
	}
	return nil
}

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
