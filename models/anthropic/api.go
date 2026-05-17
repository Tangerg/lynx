package anthropic

import (
	"context"
	"errors"
	"slices"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/Tangerg/lynx/core/model"
)

type ApiConfig struct {
	ApiKey         model.ApiKey
	RequestOptions []option.RequestOption
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("anthropic: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("anthropic: ApiKey is required")
	}
	return nil
}

type Api struct {
	client *anthropicsdk.Client
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Clone caller's slice and append the API-key option last so it
	// can't be overridden by an earlier WithAPIKey on the original
	// slice. Cloning prevents append from mutating the caller's
	// backing array when capacity allows.
	options := append(slices.Clone(cfg.RequestOptions), option.WithAPIKey(cfg.ApiKey.Get()))

	return &Api{client: new(anthropicsdk.NewClient(options...))}, nil
}

func (a *Api) ChatCompletion(ctx context.Context, req *anthropicsdk.MessageNewParams, opts ...option.RequestOption) (*anthropicsdk.Message, error) {
	if req == nil {
		return nil, errors.New("anthropic: request must not be nil")
	}
	return a.client.Messages.New(ctx, *req, opts...)
}

func (a *Api) ChatCompletionStream(ctx context.Context, req *anthropicsdk.MessageNewParams, opts ...option.RequestOption) *ssestream.Stream[anthropicsdk.MessageStreamEventUnion] {
	if req == nil {
		return nil
	}
	return a.client.Messages.NewStreaming(ctx, *req, opts...)
}

func (a *Api) CountTokens(ctx context.Context, req *anthropicsdk.MessageCountTokensParams, opts ...option.RequestOption) (*anthropicsdk.MessageTokensCount, error) {
	if req == nil {
		return nil, errors.New("anthropic: request must not be nil")
	}
	return a.client.Messages.CountTokens(ctx, *req, opts...)
}
