package openai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	RequestOptions []option.RequestOption
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("openai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

type ImageModel struct {
	api            *API
	defaultOptions image.Options
}

func NewImageModel(cfg ImageModelConfig) (*ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:         cfg.APIKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}

	return &ImageModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (i *ImageModel) buildAPIImageRequest(req *image.Request) (*openai.ImageGenerateParams, error) {
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if err := options.RejectUnsupported("openai: image", map[string]bool{
		"negative_prompt": mergedOpts.NegativePrompt != "",
		"seed":            mergedOpts.Seed != nil,
	}); err != nil {
		return nil, err
	}
	if (mergedOpts.Width == nil) != (mergedOpts.Height == nil) {
		return nil, errors.New("openai: image: width and height must be set together")
	}

	params, err := options.GetParams[openai.ImageGenerateParams](mergedOpts.Extensions, OptionsKey)
	if err != nil {
		return nil, err
	}

	params.Model = mergedOpts.Model
	params.Prompt = req.Prompt

	if mergedOpts.OutputFormat != "" {
		params.OutputFormat = openai.ImageGenerateParamsOutputFormat(strings.TrimPrefix(mergedOpts.OutputFormat, "image/"))
	}
	if mergedOpts.Width != nil && mergedOpts.Height != nil {
		params.Size = openai.ImageGenerateParamsSize(fmt.Sprintf("%dx%d", *mergedOpts.Width, *mergedOpts.Height))
	} else if params.Size == "" {
		params.Size = openai.ImageGenerateParamsSizeAuto
	}

	return params, nil
}

func (i *ImageModel) buildImageResponse(resp *openai.ImagesResponse, mimeType string) (*image.Response, error) {
	if len(resp.Data) == 0 {
		return nil, errors.New("openai: image response has no data")
	}

	results := make([]*image.Result, 0, len(resp.Data))
	for index, generated := range resp.Data {
		value, err := openAIImageMedia(mimeType, generated.URL, generated.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("openai: image %d: %w", index, err)
		}
		resultMetadata := &image.ResultMetadata{}
		if generated.RevisedPrompt != "" {
			if err := resultMetadata.Set("revised_prompt", generated.RevisedPrompt); err != nil {
				return nil, err
			}
		}
		result, err := image.NewResult(value, resultMetadata)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return image.NewResponse(results, &image.ResponseMetadata{Created: resp.Created})
}

func openAIImageMedia(mimeType, uri, encoded string) (*media.Media, error) {
	switch {
	case uri != "" && encoded == "":
		return media.NewURI(mimeType, uri)
	case uri == "" && encoded != "":
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode base64 payload: %w", err)
		}
		return media.NewBytes(mimeType, data)
	default:
		return nil, errors.New("response must contain exactly one of URL or base64 payload")
	}
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := i.buildAPIImageRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := i.api.Image(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	mimeType := "image/png"
	if apiReq.OutputFormat != "" {
		mimeType = "image/" + string(apiReq.OutputFormat)
	}
	return i.buildImageResponse(apiResp, mimeType)
}
