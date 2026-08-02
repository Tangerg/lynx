package azureopenai

import (
	"context"
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/protocol/openai"
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

var _ image.Model = (*ImageModel)(nil)

type ImageModel struct{ protocol *openai.ImageModel }

// NewImageModel returns an Azure OpenAI image model.
func NewImageModel(cfg ImageModelConfig) (*ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	reqOpts, err := buildRequestOptions(cfg.BaseURL, cfg.RequestOptions)
	if err != nil {
		return nil, err
	}
	protocol, err := openai.NewImageModel(openai.ImageModelConfig{
		Provider:       "azureopenai",
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
	if err != nil {
		return nil, err
	}
	return &ImageModel{protocol: protocol}, nil
}

func (m *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("azureopenai: nil ImageModel")
	}
	return m.protocol.Call(ctx, req)
}
