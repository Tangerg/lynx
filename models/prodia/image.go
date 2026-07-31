package prodia

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("prodia: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("prodia: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps Prodia's /v2/job inference endpoint. Model id
// ([image.Options].Model) carries the full Prodia type, e.g.
// "inference.flux.dev.txt2img.v1". Prompt-level config (negative,
// seed, sampler, steps, width, height) is threaded through the Extra
// JobRequest.Config map; the typed Width/Height/NegativePrompt/Seed
// are copied into Config automatically when set.
type ImageModel struct {
	api            *API
	defaultOptions image.Options
}

func NewImageModel(cfg ImageModelConfig) (*ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	return &ImageModel{api: api, defaultOptions: cfg.DefaultOptions.Clone()}, nil
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	apiReq, err := options.GetParams[JobRequest](mergedOpts.Extensions, ImageRequestExtensionKey)
	if err != nil {
		return nil, err
	}
	if apiReq.Type == "" {
		apiReq.Type = mergedOpts.Model
	}
	if !strings.Contains(apiReq.Type, ".txt2img.") {
		return nil, errors.New("prodia: image model requires a text-to-image job type containing .txt2img")
	}
	if apiReq.Config == nil {
		apiReq.Config = map[string]any{}
	}
	if _, ok := apiReq.Config["prompt"]; !ok {
		apiReq.Config["prompt"] = req.Prompt
	}
	if mergedOpts.NegativePrompt != "" {
		if _, ok := apiReq.Config["negative_prompt"]; !ok {
			apiReq.Config["negative_prompt"] = mergedOpts.NegativePrompt
		}
	}
	if mergedOpts.Width != nil {
		if _, ok := apiReq.Config["width"]; !ok {
			apiReq.Config["width"] = *mergedOpts.Width
		}
	}
	if mergedOpts.Height != nil {
		if _, ok := apiReq.Config["height"]; !ok {
			apiReq.Config["height"] = *mergedOpts.Height
		}
	}
	if mergedOpts.Seed != nil {
		if _, ok := apiReq.Config["seed"]; !ok {
			apiReq.Config["seed"] = *mergedOpts.Seed
		}
	}

	accept := mergedOpts.OutputFormat
	switch accept {
	case "", "image/jpeg", "image/png", "image/webp":
	default:
		return nil, errors.New("prodia: image: output_format must be image/jpeg, image/png, or image/webp")
	}
	body, hdr, err := i.api.Job(ctx, apiReq, accept)
	if err != nil {
		return nil, err
	}

	mimeType := hdr.Get("Content-Type")
	if mimeType == "" {
		mimeType = accept
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	value, err := media.NewBytes(mimeType, body)
	if err != nil {
		return nil, err
	}

	result, err := image.NewResult(value, &image.ResultMetadata{})
	if err != nil {
		return nil, err
	}

	metadata := &image.ResponseMetadata{}
	if err := metadata.Set("prodia/job_type", apiReq.Type); err != nil {
		return nil, err
	}
	return image.NewResponse([]*image.Result{result}, metadata)
}
