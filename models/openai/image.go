package openai

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/options"
	"github.com/Tangerg/lynx/pkg/mime"
)

type ImageModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *image.Options
	RequestOptions []option.RequestOption

	// Metadata overrides the [image.ModelMetadata] returned by [ImageModel.Metadata].
	// Facades pass their own Provider here. Zero falls back to [Provider].
	Metadata *image.ModelMetadata
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openai: DefaultOptions is required")
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

type ImageModel struct {
	api            *API
	defaultOptions *image.Options
	metadata       image.ModelMetadata
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

	info := image.ModelMetadata{Provider: Provider}
	if cfg.Metadata != nil {
		info = *cfg.Metadata
	}
	return &ImageModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		metadata:       info,
	}, nil
}

func (i *ImageModel) buildAPIImageRequest(req *image.Request) (*openai.ImageGenerateParams, error) {
	mergedOpts, err := image.MergeOptions(i.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := options.GetParams[openai.ImageGenerateParams](mergedOpts, OptionsKey)

	params.Model = mergedOpts.Model
	params.Prompt = req.Prompt

	if mergedOpts.OutputFormat != nil && mime.IsImage(mergedOpts.OutputFormat) {
		params.OutputFormat = openai.ImageGenerateParamsOutputFormat(mergedOpts.OutputFormat.SubType())
	}
	if mergedOpts.ResponseFormat.Valid() {
		params.ResponseFormat = openai.ImageGenerateParamsResponseFormat(mergedOpts.ResponseFormat)
	}
	if mergedOpts.Width != nil && mergedOpts.Height != nil {
		params.Size = openai.ImageGenerateParamsSize(fmt.Sprintf("%dx%d", *mergedOpts.Width, *mergedOpts.Height))
	} else if params.Size == "" {
		params.Size = openai.ImageGenerateParamsSizeAuto
	}

	if mergedOpts.Style != "" {
		params.Style = openai.ImageGenerateParamsStyle(mergedOpts.Style)
	}

	if mergedOpts.Quality != "" {
		params.Quality = openai.ImageGenerateParamsQuality(mergedOpts.Quality)
	}

	return params, nil
}

func (i *ImageModel) buildImageResponse(resp *openai.ImagesResponse) (*image.Response, error) {
	// The image surface is single-result by design (see image.Response);
	// callers wanting N>1 drop down to the openai-go SDK directly.
	if len(resp.Data) == 0 {
		return nil, errors.New("openai: image response has no data")
	}

	img, err := image.NewImage(resp.Data[0].URL, resp.Data[0].B64JSON)
	if err != nil {
		return nil, err
	}

	result, err := image.NewResult(img, &image.ResultMetadata{})
	if err != nil {
		return nil, err
	}

	return image.NewResponse(result, &image.ResponseMetadata{Created: resp.Created})
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	apiReq, err := i.buildAPIImageRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := i.api.Image(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return i.buildImageResponse(apiResp)
}

func (i *ImageModel) DefaultOptions() image.Options {
	return *i.defaultOptions
}

func (i *ImageModel) Metadata() image.ModelMetadata {
	return i.metadata
}
