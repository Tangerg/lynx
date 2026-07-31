package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/openai"
)

type ImageModelConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions image.Options
	RequestOptions []option.RequestOption
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

// NewImageModel returns an [openai.ImageModel] pointed at Azure
// OpenAI's v1 /images/generations endpoint. [image.Options].Model is
// the Azure deployment id (typically pointing at "dall-e-3" or
// "gpt-image-1").
func NewImageModel(cfg ImageModelConfig) (*openai.ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	reqOpts, err := buildRequestOptions(cfg.BaseURL, cfg.RequestOptions)
	if err != nil {
		return nil, err
	}
	return openai.NewImageModel(openai.ImageModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
}
