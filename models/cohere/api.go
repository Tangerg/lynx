package cohere

import (
	"context"
	"errors"

	cohere "github.com/cohere-ai/cohere-go/v2"
	"github.com/cohere-ai/cohere-go/v2/core"
	cohereoption "github.com/cohere-ai/cohere-go/v2/option"
	cohereclientv2 "github.com/cohere-ai/cohere-go/v2/v2"

	"github.com/Tangerg/lynx/core/model"
)

type ApiConfig struct {
	ApiKey         model.ApiKey
	RequestOptions []cohereoption.RequestOption
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("cohere: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("cohere: ApiKey is required")
	}
	return nil
}

// Api wraps Cohere's v2 client. Only Embed is surfaced by design:
// Cohere's chat ecosystem (documents, citations, web_search, ...) does
// not map cleanly onto the OpenAI-shaped chat surface, and Cohere's
// chat models lag behind OpenAI/Anthropic/Google in capability. For
// embeddings — multilingual retrieval especially — Cohere remains
// competitive, so that's the slice we expose.
type Api struct {
	v2 *cohereclientv2.Client
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// v2 client takes a core.RequestOptions struct (not functional
	// options). Build one with our token; callers wanting custom
	// HTTPClient / BaseURL should reach for the [cohereclientv2.NewClient]
	// directly in a follow-up.
	reqOpts := &core.RequestOptions{
		Token: cfg.ApiKey.Get(),
	}

	return &Api{v2: cohereclientv2.NewClient(reqOpts)}, nil
}

func (a *Api) Embed(ctx context.Context, req *cohere.V2EmbedRequest, opts ...cohereoption.RequestOption) (*cohere.EmbedByTypeResponse, error) {
	if req == nil {
		return nil, errors.New("cohere: request must not be nil")
	}
	return a.v2.Embed(ctx, req, opts...)
}
