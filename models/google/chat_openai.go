package google

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

	// BaseURL defaults to [BaseURLOpenAI]. The bridge is hosted
	// under Generative Language's v1beta surface; Vertex AI exposes
	// a separate path (use Vertex-specific tooling for that).
	BaseURL string

	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c OpenAIChatModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("google: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("google: DefaultOptions is required")
	}
	return nil
}

// NewOpenAIChatModel returns an openai-backed [openai.ChatModel]
// pointed at Gemini's first-party OpenAI-compatible endpoint. Use
// this constructor to keep an OpenAI-SDK integration intact while
// targeting Gemini models; for native genai surfaces (caches,
// thinking budget, safety settings, structured grounding) use
// [NewChatModel] instead.
//
// Note: the bridge is wire-format-only — Gemini-specific fields not
// in the OpenAI schema (system instructions on cache, safety
// thresholds, response modalities) are not exposed through this
// path.
func NewOpenAIChatModel(cfg OpenAIChatModelConfig) (*openai.ChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, BaseURLOpenAI)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return openai.NewChatModel(openai.ChatModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
