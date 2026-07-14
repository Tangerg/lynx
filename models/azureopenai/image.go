package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/openai"
)

type ImageModelConfig struct {
	APIKey         string
	Endpoint       string
	APIVersion     string
	DefaultOptions *image.Options
	RequestOptions []option.RequestOption
}

func (c ImageModelConfig) Validate() error {
	if c.Endpoint == "" {
		return errors.New("azureopenai: Endpoint is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("azureopenai: DefaultOptions is required")
	}
	return nil
}

// NewImageModel returns an [openai.ImageModel] pointed at Azure
// OpenAI's /images/generations endpoint. [image.Options].Model is
// the Azure deployment id (typically pointing at "dall-e-3" or
// "gpt-image-1").
func NewImageModel(cfg ImageModelConfig) (*openai.ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	apiKey, reqOpts := buildAzureRequestOptions(cfg.APIKey, cfg.Endpoint, cfg.APIVersion, cfg.RequestOptions)
	return openai.NewImageModel(openai.ImageModelConfig{
		APIKey:         apiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
}
