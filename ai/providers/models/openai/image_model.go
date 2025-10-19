package openai

import (
	"context"
	"errors"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/image"
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

// buildApiImageRequest Todo Complete all parameters
func (i *ImageModel) buildApiImageRequest(req *image.Request) (*openai.ImageGenerateParams, error) {
	mergedOpts, err := image.MergeOptions(i.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}
	apiParams := &openai.ImageGenerateParams{
		Prompt: req.Prompt,
		Model:  mergedOpts.Model,
	}
	return apiParams, nil
}

// buildImageResp Todo Complete all parameters
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
		Provider: provider,
	}
}
