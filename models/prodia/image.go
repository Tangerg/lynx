package prodia

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *image.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("prodia: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("prodia: DefaultOptions is required")
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
	defaultOptions *image.Options
}

func NewImageModel(cfg ImageModelConfig) (*ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	return &ImageModel{api: api, defaultOptions: cfg.DefaultOptions}, nil
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	mergedOpts, err := image.MergeOptions(i.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq := options.GetParams[JobRequest](mergedOpts, OptionsKey)
	if apiReq.Type == "" {
		apiReq.Type = mergedOpts.Model
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

	body, hdr, err := i.api.Job(ctx, apiReq, "")
	if err != nil {
		return nil, err
	}

	img, err := image.NewImage("", base64.StdEncoding.EncodeToString(body))
	if err != nil {
		return nil, err
	}

	resultMeta := &image.ResultMetadata{}
	if ct := hdr.Get("Content-Type"); ct != "" {
		resultMeta.Set("mime_type", ct)
	}

	result, err := image.NewResult(img, resultMeta)
	if err != nil {
		return nil, err
	}

	return image.NewResponse(result, &image.ResponseMetadata{Created: time.Now().Unix()})
}

func (i *ImageModel) DefaultOptions() image.Options { return *i.defaultOptions }
func (i *ImageModel) Metadata() image.ModelMetadata { return image.ModelMetadata{Provider: Provider} }
