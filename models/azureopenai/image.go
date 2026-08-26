package azureopenai

import (
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/protocol/openai"
)

type ImageModelConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions image.Options
	HTTPClient     *http.Client
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("azureopenai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

type ImageModel = openai.ImageModel

// NewImageModel returns an Azure OpenAI image model.
func NewImageModel(cfg ImageModelConfig) (*ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return openai.NewImageModel(openai.ImageModelConfig{
		Provider:       "azureopenai",
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		BaseURL:        baseURL,
		HTTPClient:     cfg.HTTPClient,
	})
}
