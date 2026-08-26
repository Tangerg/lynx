package anthropic

import (
	"context"
	"errors"
	"net/http"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

type apiConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	Headers    http.Header
}

func (c apiConfig) validate() error {
	if c.APIKey == "" {
		return errors.New("anthropic: APIKey is required")
	}
	return nil
}

type api struct {
	client *anthropicsdk.Client
}

func newAPI(cfg apiConfig) (*api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	options := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		options = append(options, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		options = append(options, option.WithHTTPClient(cfg.HTTPClient))
	}
	for name, values := range cfg.Headers {
		for _, value := range values {
			options = append(options, option.WithHeader(name, value))
		}
	}
	client := anthropicsdk.NewClient(options...)

	return &api{client: &client}, nil
}

func (a *api) chatCompletion(ctx context.Context, req *anthropicsdk.MessageNewParams) (*anthropicsdk.Message, error) {
	if req == nil {
		return nil, errors.New("anthropic: request must not be nil")
	}
	return a.wrapResult(a.client.Messages.New(ctx, *req))
}

func (a *api) chatCompletionStream(ctx context.Context, req *anthropicsdk.MessageNewParams) *ssestream.Stream[anthropicsdk.MessageStreamEventUnion] {
	if req == nil {
		return nil
	}
	return a.client.Messages.NewStreaming(ctx, *req)
}

func (a *api) countTokens(ctx context.Context, req *anthropicsdk.MessageCountTokensParams) (*anthropicsdk.MessageTokensCount, error) {
	if req == nil {
		return nil, errors.New("anthropic: request must not be nil")
	}
	return a.wrapResult(a.client.Messages.CountTokens(ctx, *req))
}
