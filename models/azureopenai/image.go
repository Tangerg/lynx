package azureopenai

import (
	"errors"
	"net/http"

	"github.com/Tangerg/scope/core/image"
	"github.com/Tangerg/scope/models/protocol/openai"
)

type ImageModelConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions image.Options
	HTTPClient     *http.Client
}

func (i ImageModelConfig) Validate() error {
	if i.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
	}
	if i.DefaultOptions.Model == "" {
		return errors.New("azureopenai: DefaultOptions.Model is required")
	}
	if err := i.DefaultOptions.Validate(); err != nil {
		return err
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

type ImageModel = openai.ImageModel

func NewImageModel(config ImageModelConfig) (*ImageModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := normalizeBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	return openai.NewImageModel(openai.ImageModelConfig{
		Provider:       "azureopenai",
		APIKey:         config.APIKey,
		DefaultOptions: config.DefaultOptions,
		BaseURL:        baseURL,
		HTTPClient:     config.HTTPClient,
	})
}
