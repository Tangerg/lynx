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

type APIConfig struct {
	APIKey         model.APIKey
	RequestOptions []option.RequestOption
}

func (c *APIConfig) validate() error {
	if c == nil {
		return errors.New("anthropic: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("anthropic: APIKey is required")
	}
	return nil
}

type API struct {
	client *anthropicsdk.Client
}

func NewAPI(cfg *APIConfig) (*API, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Clone caller's slice and append the API-key option last so it
	// can't be overridden by an earlier WithAPIKey on the original
	// slice. Cloning prevents append from mutating the caller's
	// backing array when capacity allows.
	options := append(slices.Clone(cfg.RequestOptions), option.WithAPIKey(cfg.APIKey.Get()))

	return &API{client: new(anthropicsdk.NewClient(options...))}, nil
}

func (a *API) ChatCompletion(ctx context.Context, req *anthropicsdk.MessageNewParams, opts ...option.RequestOption) (*anthropicsdk.Message, error) {
	if req == nil {
		return nil, errors.New("anthropic: request must not be nil")
	}
	return a.client.Messages.New(ctx, *req, opts...)
}

func (a *API) ChatCompletionStream(ctx context.Context, req *anthropicsdk.MessageNewParams, opts ...option.RequestOption) *ssestream.Stream[anthropicsdk.MessageStreamEventUnion] {
	if req == nil {
		return nil
	}
	return a.client.Messages.NewStreaming(ctx, *req, opts...)
}

func (a *API) CountTokens(ctx context.Context, req *anthropicsdk.MessageCountTokensParams, opts ...option.RequestOption) (*anthropicsdk.MessageTokensCount, error) {
	if req == nil {
		return nil, errors.New("anthropic: request must not be nil")
	}
	return a.client.Messages.CountTokens(ctx, *req, opts...)
}
