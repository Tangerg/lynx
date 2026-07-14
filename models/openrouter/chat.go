package openrouter

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
	BaseURL        string

	// AppURL populates the HTTP-Referer header. OpenRouter uses it
	// for app attribution / analytics. Optional but recommended.
	AppURL string

	// AppTitle populates the X-Title header. OpenRouter shows this
	// on the leaderboard rankings. Optional but recommended.
	AppTitle string

	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c OpenAIChatModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("openrouter: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openrouter: DefaultOptions is required")
	}
	return nil
}

// NewOpenAIChatModel returns an openai-backed chat model pointed at
// OpenRouter. App-attribution headers (HTTP-Referer / X-Title) are
// injected when configured.
//
// OpenRouter-specific request fields (models[] for fallback, provider
// preferences, transforms, route) ride through Extra-threaded openai
// params; they aren't typed in the openai-go SDK but the API accepts
// them as additional JSON fields and the SDK forwards them unchanged.
// For the Anthropic-shaped /v1/messages endpoint use
// [NewAnthropicChatModel] instead.
func NewOpenAIChatModel(cfg OpenAIChatModelConfig) (*openai.ChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	baseURL := cmp.Or(cfg.BaseURL, BaseURL)
	reqOpts := []option.RequestOption{option.WithBaseURL(baseURL)}
	if cfg.AppURL != "" {
		reqOpts = append(reqOpts, option.WithHeader(HeaderReferer, cfg.AppURL))
	}
	if cfg.AppTitle != "" {
		reqOpts = append(reqOpts, option.WithHeader(HeaderAppTitle, cfg.AppTitle))
	}
	reqOpts = append(reqOpts, cfg.RequestOptions...)

	return openai.NewChatModel(openai.ChatModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
