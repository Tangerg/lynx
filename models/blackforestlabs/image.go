package blackforestlabs

import (
	"context"
	"errors"
	"fmt"
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

	// PollInterval / PollTimeout configure the synchronous wrapper
	// around BFL's async generation. Zero values fall back to the
	// package defaults.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("blackforestlabs: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("blackforestlabs: DefaultOptions is required")
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps Black Forest Labs' Flux image-generation endpoints.
// Model id picks the engine ("flux-pro-1.1", "flux-pro-1.1-ultra",
// "flux-dev", "flux-kontext-pro", "flux-kontext-max"). BFL is async
// only — Call submits + polls until ready.
type ImageModel struct {
	api            *API
	defaultOptions *image.Options
	pollInterval   time.Duration
	pollTimeout    time.Duration
}

func NewImageModel(cfg ImageModelConfig) (*ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	pi := cfg.PollInterval
	if pi <= 0 {
		pi = time.Duration(DefaultPollIntervalSeconds) * time.Second
	}
	pt := cfg.PollTimeout
	if pt <= 0 {
		pt = time.Duration(DefaultPollTimeoutSeconds) * time.Second
	}
	return &ImageModel{api: api, defaultOptions: cfg.DefaultOptions, pollInterval: pi, pollTimeout: pt}, nil
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	mergedOpts, err := image.MergeOptions(i.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq := options.GetParams[GenerateRequest](mergedOpts, OptionsKey)
	apiReq.Prompt = req.Prompt
	if mergedOpts.Width != nil {
		apiReq.Width = int(*mergedOpts.Width)
	}
	if mergedOpts.Height != nil {
		apiReq.Height = int(*mergedOpts.Height)
	}
	if mergedOpts.Seed != nil {
		apiReq.Seed = mergedOpts.Seed
	}
	if mergedOpts.OutputFormat != nil && apiReq.OutputFormat == "" {
		apiReq.OutputFormat = mergedOpts.OutputFormat.SubType()
	}

	async, err := i.api.Generate(ctx, mergedOpts.Model, apiReq)
	if err != nil {
		return nil, err
	}

	final, err := i.pollUntilDone(ctx, async.ID)
	if err != nil {
		return nil, err
	}

	img, err := image.NewImage(final.Result.Sample, "")
	if err != nil {
		return nil, err
	}

	resultMeta := &image.ResultMetadata{}
	if final.Result.Seed != 0 {
		resultMeta.Set("seed", final.Result.Seed)
	}
	if final.Result.Duration != 0 {
		resultMeta.Set("duration_ms", final.Result.Duration)
	}

	result, err := image.NewResult(img, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &image.ResponseMetadata{Created: time.Now().Unix()}
	meta.Set("task_id", async.ID)
	return image.NewResponse(result, meta)
}

func (i *ImageModel) pollUntilDone(ctx context.Context, id string) (*PollResult, error) {
	deadline, cancel := context.WithTimeout(ctx, i.pollTimeout)
	defer cancel()

	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()

	for {
		resp, err := i.api.GetResult(deadline, id)
		if err != nil {
			return nil, err
		}
		switch resp.Status {
		case "Ready":
			return resp, nil
		case "Error", "Content Moderated", "Request Moderated", "Task not found":
			return nil, fmt.Errorf("blackforestlabs: generation failed: %s", resp.Status)
		}
		select {
		case <-deadline.Done():
			return nil, deadline.Err()
		case <-ticker.C:
		}
	}
}

func (i *ImageModel) DefaultOptions() image.Options { return *i.defaultOptions }
func (i *ImageModel) Metadata() image.ModelMetadata         { return image.ModelMetadata{Provider: Provider} }
