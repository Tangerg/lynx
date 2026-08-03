package luma

import (
	"cmp"
	"context"
	"errors"
	"net/http"

	lumaagents "github.com/lumalabs/luma-agents-go"
	lumaoption "github.com/lumalabs/luma-agents-go/option"
)

type apiConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (config apiConfig) validate() error {
	if config.APIKey == "" {
		return errors.New("luma: APIKey is required")
	}
	return nil
}

type api struct {
	client *lumaagents.Client
}

func newAPI(config apiConfig) (*api, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	requestOptions := []lumaoption.RequestOption{
		lumaoption.WithAuthToken(config.APIKey),
		lumaoption.WithBaseURL(cmp.Or(config.BaseURL, DefaultBaseURL)),
	}
	if config.HTTPClient != nil {
		requestOptions = append(requestOptions, lumaoption.WithHTTPClient(config.HTTPClient))
	}
	return &api{client: lumaagents.NewClient(requestOptions...)}, nil
}

func (api *api) createGeneration(ctx context.Context, params lumaagents.GenerationNewParams) (*lumaagents.Generation, error) {
	if api == nil || api.client == nil || api.client.Generations == nil {
		return nil, errors.New("luma: nil API")
	}
	return api.client.Generations.New(ctx, params)
}

func (api *api) getGeneration(ctx context.Context, generationID string) (*lumaagents.Generation, error) {
	if api == nil || api.client == nil || api.client.Generations == nil {
		return nil, errors.New("luma: nil API")
	}
	if generationID == "" {
		return nil, errors.New("luma: generation ID is required")
	}
	return api.client.Generations.Get(ctx, generationID)
}
