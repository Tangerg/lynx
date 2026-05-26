package huggingface

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

func (c *OpenAIChatModelConfig) validate() error {
	if c == nil {
		return errors.New("huggingface: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("huggingface: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("huggingface: DefaultOptions is required")
	}
	return nil
}

// NewOpenAIChatModel builds a chat model backed by the HuggingFace
// Inference Router. The router accepts model ids in
// "provider/model-name" format (e.g.
// "meta-llama/Llama-3.1-8B-Instruct:fireworks-ai") which the caller
// supplies through [chat.Options].Model.
func NewOpenAIChatModel(cfg *OpenAIChatModelConfig) (*openai.ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	baseURL := cmp.Or(cfg.BaseURL, DefaultBaseURL)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)

	return openai.NewChatModel(&openai.ChatModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
