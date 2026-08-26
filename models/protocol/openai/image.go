package openai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
)

type ImageModelConfig struct {
	Provider       string
	APIKey         string
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (i ImageModelConfig) Validate() error {
	if err := validateProvider(i.Provider); err != nil {
		return fmt.Errorf("openai: Provider: %w", err)
	}
	if i.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if i.DefaultOptions.Model == "" {
		return errors.New("openai: DefaultOptions.Model is required")
	}
	if _, err := i.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

type ImageModel struct {
	api            *api
	provider       string
	defaultOptions image.Options
}

func NewImageModel(cfg ImageModelConfig) (*ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := newAPI(apiConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &ImageModel{
		api:            api,
		provider:       cfg.Provider,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (i *ImageModel) buildAPIImageRequest(req *image.Request) (*openai.ImageGenerateParams, error) {
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if rejectUnsupportedOptionsErr := rejectUnsupportedOptions("openai: image", map[string]bool{
		"negative_prompt": mergedOpts.NegativePrompt != "",
		"seed":            mergedOpts.Seed != nil,
	}); rejectUnsupportedOptionsErr != nil {
		return nil, rejectUnsupportedOptionsErr
	}
	if (mergedOpts.Width == nil) != (mergedOpts.Height == nil) {
		return nil, errors.New("openai: image: width and height must be set together")
	}

	fields, err := decodeRequestFields(mergedOpts.Extensions, protocolModalityRequestExtensionKey(i.provider, "image"), "model", "prompt", "output_format", "size")
	if err != nil {
		return nil, err
	}
	params := &openai.ImageGenerateParams{}
	params.SetExtraFields(fields)

	params.Model = mergedOpts.Model
	params.Prompt = req.Prompt

	if mergedOpts.OutputFormat != "" {
		params.OutputFormat = openai.ImageGenerateParamsOutputFormat(strings.TrimPrefix(mergedOpts.OutputFormat, "image/"))
	}
	if mergedOpts.Width != nil && mergedOpts.Height != nil {
		params.Size = openai.ImageGenerateParamsSize(fmt.Sprintf("%dx%d", *mergedOpts.Width, *mergedOpts.Height))
	} else {
		params.Size = openai.ImageGenerateParamsSizeAuto
	}

	return params, nil
}

func (i *ImageModel) buildImageResponse(resp *openai.ImagesResponse, mimeType string) (*image.Response, error) {
	if len(resp.Data) == 0 {
		return nil, errors.New("openai: image response has no data")
	}

	outputs := make([]*image.Output, 0, len(resp.Data))
	for index, generated := range resp.Data {
		value, err := openAIImageMedia(mimeType, generated.URL, generated.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("openai: image %d: %w", index, err)
		}
		outputMetadata := &image.OutputMetadata{}
		if generated.RevisedPrompt != "" {
			if setErr := outputMetadata.Set(i.provider+"/revised_prompt", generated.RevisedPrompt); setErr != nil {
				return nil, setErr
			}
		}
		output, err := image.NewOutput(value, outputMetadata)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}

	return image.NewResponse(outputs, &image.ResponseMetadata{Created: resp.Created})
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

	apiResp, err := i.api.image(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	mimeType := "image/png"
	if apiReq.OutputFormat != "" {
		mimeType = "image/" + string(apiReq.OutputFormat)
	}
	return i.buildImageResponse(apiResp, mimeType)
}
