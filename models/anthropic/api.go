package anthropic

import (
	"context"
	"errors"

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
		return errors.New("config is nil")
	}
	if c.ApiKey == nil {
		return errors.New("apiKey is required")
	}
	return nil
}

type Api struct {
	apiKey model.ApiKey
	client *anthropicsdk.Client
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// ensure apikey at last
	options := append(cfg.RequestOptions, option.WithAPIKey(cfg.ApiKey.Get()))
	client := anthropicsdk.NewClient(options...)

	return &Api{
		apiKey: cfg.ApiKey,
		client: &client,
	}, nil
}

func (a *Api) ChatCompletion(ctx context.Context, req *anthropicsdk.MessageNewParams, opts ...option.RequestOption) (*anthropicsdk.Message, error) {
	if req == nil {
		return nil, errors.New("request parameters cannot be nil")
	}
	return a.client.Messages.New(ctx, *req, opts...)
}

func (a *Api) ChatCompletionStream(ctx context.Context, req *anthropicsdk.MessageNewParams, opts ...option.RequestOption) *ssestream.Stream[anthropicsdk.MessageStreamEventUnion] {
	if req == nil {
		return nil
	}
	return a.client.Messages.NewStreaming(ctx, *req, opts...)
}
