package openai

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/image"
	"github.com/Tangerg/lynx/pkg/mime"
)

var _ image.Model = (*ImageModel)(nil)

type ImageModel struct {
	api            *Api
	defaultOptions *image.Options
}

func NewImageModel(apiKey model.ApiKey, defaultOptions *image.Options, opts ...option.RequestOption) (*ImageModel, error) {
	if defaultOptions == nil {
		return nil, errors.New("default options cannot be nil")
	}

	api, err := NewApi(apiKey, opts...)
	if err != nil {
		return nil, err
	}

	return &ImageModel{
		api:            api,
		defaultOptions: defaultOptions,
	}, nil
}

func (i *ImageModel) buildApiImageRequest(req *image.Request) (*openai.ImageGenerateParams, error) {
	mergedOpts, err := image.MergeOptions(i.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}
	params := getOptionsParams[openai.ImageGenerateParams](mergedOpts)

	params.Prompt = req.Prompt

	params.Model = mergedOpts.Model

	if mergedOpts.OutputFormat != nil &&
		mime.IsImage(mergedOpts.OutputFormat) {
		params.OutputFormat = openai.ImageGenerateParamsOutputFormat(mergedOpts.OutputFormat.SubType())
	}

	if mergedOpts.ResponseFormat.Valid() {
		params.ResponseFormat = openai.ImageGenerateParamsResponseFormat(mergedOpts.ResponseFormat)
	}

	if mergedOpts.Width != nil && mergedOpts.Height != nil {
		params.Size = openai.ImageGenerateParamsSize(fmt.Sprintf("%dx%d", mergedOpts.Width, mergedOpts.Height))
	} else {
		params.Size = openai.ImageGenerateParamsSizeAuto
	}

	params.Style = openai.ImageGenerateParamsStyle(mergedOpts.Style)

	return params, nil
}

func (i *ImageModel) buildImageResp(resp *openai.ImagesResponse) (*image.Response, error) {
	results := make([]*image.Result, 0, len(resp.Data))
	for _, item := range resp.Data {
		newImage, err := image.NewImage(item.URL, item.B64JSON)
		if err != nil {
			return nil, err
		}
		result, err := image.NewResult(newImage, &image.ResultMetadata{})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	respMeta := &image.ResponseMetadata{
		Created: resp.Created,
	}
	return image.NewResponse(results, respMeta)
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	apiReq, err := i.buildApiImageRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := i.api.Images(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return i.buildImageResp(apiResp)
}

func (i *ImageModel) DefaultOptions() *image.Options {
	return i.defaultOptions
}

func (i *ImageModel) Info() image.ModelInfo {
	return image.ModelInfo{
		Provider: Provider,
	}
}
