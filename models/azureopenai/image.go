package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/openai"
)

type ImageModelConfig struct {
	ApiKey         model.ApiKey
	Endpoint       string
	APIVersion     string
	DefaultOptions *image.Options
	RequestOptions []option.RequestOption
}

func (c *ImageModelConfig) validate() error {
	if c == nil {
		return errors.New("azureopenai: config must not be nil")
	}
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
func NewImageModel(cfg *ImageModelConfig) (*openai.ImageModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	apiKey, reqOpts := buildAzureRequestOptions(cfg.ApiKey, cfg.Endpoint, cfg.APIVersion, cfg.RequestOptions)
	return openai.NewImageModel(&openai.ImageModelConfig{
		ApiKey:         apiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &image.ModelMetadata{Provider: Provider},
	})
}
