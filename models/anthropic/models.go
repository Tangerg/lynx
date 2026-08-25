package anthropic

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	corechat "github.com/Tangerg/lynx/core/chat"
	anthropicprotocol "github.com/Tangerg/lynx/models/internal/protocol/anthropic"
	openaiprotocol "github.com/Tangerg/lynx/models/protocol/openai"
)

const (
	Provider      = "Anthropic"
	BaseURLOpenAI = "https://api.anthropic.com/v1"

	OpenAIRequestExtensionKey     = "anthropic/openai_request"
	OpenAIResponseExtensionKey    = "anthropic/openai_response"
	OpenAIStreamChunkExtensionKey = "anthropic/openai_stream_chunk"
)

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error { return c.protocol().Validate() }

func (c ChatConfig) protocol() anthropicprotocol.ChatConfig {
	return anthropicprotocol.ChatConfig{APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

type Chat struct{ protocol *anthropicprotocol.Chat }

func NewChat(config ChatConfig) (*Chat, error) {
	model, err := anthropicprotocol.NewChat(config.protocol())
	if err != nil {
		return nil, err
	}
	return &Chat{protocol: model}, nil
}

func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	if c == nil || c.protocol == nil {
		return nil, errors.New("anthropic: nil Chat")
	}
	return c.protocol.Call(ctx, req)
}

func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if c == nil || c.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("anthropic: nil Chat")) }
	}
	return c.protocol.Stream(ctx, req)
}

type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (config OpenAIChatConfig) Validate() error {
	if config.APIKey == "" {
		return errors.New("anthropic: APIKey is required")
	}
	if err := config.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("anthropic: DefaultOptions: %w", err)
	}
	return nil
}

type OpenAIChat struct{ protocol *openaiprotocol.Chat }

func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	model, err := openaiprotocol.NewCompatibleChat(
		openaiprotocol.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLOpenAI), HTTPClient: config.HTTPClient},
		openaiprotocol.Dialect{Provider: "anthropic"},
	)
	if err != nil {
		return nil, fmt.Errorf("anthropic: construct OpenAI-compatible chat: %w", err)
	}
	return &OpenAIChat{protocol: model}, nil
}

func (c *OpenAIChat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	if c == nil || c.protocol == nil {
		return nil, errors.New("anthropic: nil OpenAIChat")
	}
	return c.protocol.Call(ctx, req)
}

func (c *OpenAIChat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if c == nil || c.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("anthropic: nil OpenAIChat")) }
	}
	return c.protocol.Stream(ctx, req)
}

type TextEstimatorConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

func (c TextEstimatorConfig) Validate() error { return c.protocol().Validate() }

func (c TextEstimatorConfig) protocol() anthropicprotocol.TextEstimatorConfig {
	return anthropicprotocol.TextEstimatorConfig{APIKey: c.APIKey, Model: c.Model, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

type TextEstimator struct {
	protocol *anthropicprotocol.TextEstimator
}

func NewTextEstimator(config TextEstimatorConfig) (*TextEstimator, error) {
	estimator, err := anthropicprotocol.NewTextEstimator(config.protocol())
	if err != nil {
		return nil, err
	}
	return &TextEstimator{protocol: estimator}, nil
}

func (e *TextEstimator) EstimateText(ctx context.Context, value string) (int, error) {
	if e == nil || e.protocol == nil {
		return 0, errors.New("anthropic: nil TextEstimator")
	}
	return e.protocol.EstimateText(ctx, value)
}
