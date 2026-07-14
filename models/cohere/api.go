package cohere

import (
	"context"
	"errors"

	cohere "github.com/cohere-ai/cohere-go/v2"
	"github.com/cohere-ai/cohere-go/v2/core"
	cohereoption "github.com/cohere-ai/cohere-go/v2/option"
	cohereclientv2 "github.com/cohere-ai/cohere-go/v2/v2"
)

type APIConfig struct {
	APIKey  string
	BaseURL string

	RequestOptions []cohereoption.RequestOption
}

func (c APIConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("cohere: APIKey is required")
	}
	return nil
}

// API wraps Cohere's v2 client. Only Embed is surfaced by design:
// Cohere's chat ecosystem (documents, citations, web_search, ...) does
// not map cleanly onto the OpenAI-shaped chat surface, and Cohere's
// chat models lag behind OpenAI/Anthropic/Google in capability. For
// embeddings — multilingual retrieval especially — Cohere remains
// competitive, so that's the slice exposed here.
type API struct {
	v2 *cohereclientv2.Client
}

func NewAPI(cfg APIConfig) (*API, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// v2 client takes a core.RequestOptions struct (not functional
	// options). Build one with our token + caller-supplied BaseURL.
	// Per-call options can still be passed at Embed time.
	reqOpts := &core.RequestOptions{
		Token:   cfg.APIKey,
		BaseURL: cfg.BaseURL,
	}

	return &API{v2: cohereclientv2.NewClient(reqOpts)}, nil
}

func (a *API) Embed(ctx context.Context, req *cohere.V2EmbedRequest, opts ...cohereoption.RequestOption) (*cohere.EmbedByTypeResponse, error) {
	if req == nil {
		return nil, errors.New("cohere: request must not be nil")
	}
	return a.v2.Embed(ctx, req, opts...)
}
