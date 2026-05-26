package ollama

import (
	"cmp"
	"errors"
	"strings"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/openai"
)

type OpenAIChatModelConfig struct {
	// APIKey is optional — local Ollama daemons don't enforce auth.
	// Provide one only when running behind a gateway / reverse proxy
	// that requires Bearer auth. nil sends an empty Bearer.
	APIKey model.APIKey

	DefaultOptions *chat.Options

	// BaseURL points at the Ollama daemon. The "/v1" suffix is appended
	// automatically when missing. Empty falls back to
	// [DefaultBaseURL] + [OpenAICompatPath].
	BaseURL string

	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c *OpenAIChatModelConfig) validate() error {
	if c == nil {
		return errors.New("ollama: config must not be nil")
	}
	if c.DefaultOptions == nil {
		return errors.New("ollama: DefaultOptions is required")
	}
	return nil
}

// NewOpenAIChatModel returns an openai-backed chat model pointed at
// Ollama's /v1/chat/completions endpoint. Pick this constructor when
// the caller wants the OpenAI surface (response_format, tool calling
// via openai-shaped tool definitions, reasoning_content auto-routing);
// use [NewNativeChatModel] for the Ollama-native API instead.
//
// Note: Ollama's OpenAI-compatible mode does NOT support every native
// feature — Ollama-specific knobs (keep_alive, format=json shorthand,
// num_predict, mirostat, ...) are only on the native surface.
func NewOpenAIChatModel(cfg *OpenAIChatModelConfig) (*openai.ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	baseURL := resolveOpenAIBaseURL(cfg.BaseURL)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	// openai.ChatModelConfig requires a non-nil APIKey; supply an
	// empty placeholder when the local Ollama daemon doesn't enforce
	// auth.
	apiKey := cfg.APIKey
	if apiKey == nil {
		apiKey = model.NewAPIKey("ollama")
	}
	return openai.NewChatModel(&openai.ChatModelConfig{
		APIKey:         apiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}

// resolveOpenAIBaseURL joins the configured host with [OpenAICompatPath]
// when the caller passed a bare daemon address (no /v1 suffix). Lets
// users write either "http://localhost:11434" or
// "http://localhost:11434/v1" and get the right result either way.
func resolveOpenAIBaseURL(base string) string {
	base = strings.TrimRight(cmp.Or(base, DefaultBaseURL), "/")
	if strings.HasSuffix(base, OpenAICompatPath) {
		return base
	}
	return base + OpenAICompatPath
}
