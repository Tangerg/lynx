package luma

import (
	"cmp"
	"context"
	"errors"
	"net/http"

	lumaagents "github.com/lumalabs/luma-agents-go"
	lumaoption "github.com/lumalabs/luma-agents-go/option"
)

type APIConfig struct {
	APIKey         string
	BaseURL        string
	HTTPClient     *http.Client
	RequestOptions []lumaoption.RequestOption
}

func (config APIConfig) Validate() error {
	if config.APIKey == "" {
		return errors.New("luma: APIKey is required")
	}
	return nil
}

// API is a narrow wrapper around Luma's official Agents SDK.
type API struct {
	client *lumaagents.Client
}

func NewAPI(config APIConfig) (*API, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	requestOptions := []lumaoption.RequestOption{
		lumaoption.WithAuthToken(config.APIKey),
		lumaoption.WithBaseURL(cmp.Or(config.BaseURL, DefaultBaseURL)),
	}
	if config.HTTPClient != nil {
		requestOptions = append(requestOptions, lumaoption.WithHTTPClient(config.HTTPClient))
	}
	requestOptions = append(requestOptions, config.RequestOptions...)
	return &API{client: lumaagents.NewClient(requestOptions...)}, nil
}

func (api *API) CreateGeneration(ctx context.Context, params lumaagents.GenerationNewParams) (*lumaagents.Generation, error) {
	if api == nil || api.client == nil || api.client.Generations == nil {
		return nil, errors.New("luma: nil API")
	}
	return api.client.Generations.New(ctx, params)
}

func (api *API) GetGeneration(ctx context.Context, generationID string) (*lumaagents.Generation, error) {
	if api == nil || api.client == nil || api.client.Generations == nil {
		return nil, errors.New("luma: nil API")
	}
	if generationID == "" {
		return nil, errors.New("luma: generation ID is required")
	}
	return api.client.Generations.Get(ctx, generationID)
}
