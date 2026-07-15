package luma

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions *image.Options
	BaseURL        string
	HTTPClient     *http.Client
	PollInterval   time.Duration
	PollTimeout    time.Duration
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("luma: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("luma: DefaultOptions is required")
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps Luma's Photon image-generation endpoint
// (/dream-machine/v1/generations/image). Luma is async — Call submits
// then polls until the asset is ready.
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
	api, err := NewAPI(APIConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
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
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := image.MergeOptions(i.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[ImageGenerateRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}
	apiReq.Prompt = req.Prompt
	if apiReq.Model == "" {
		apiReq.Model = mergedOpts.Model
	}

	async, err := i.api.GenerateImage(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	final, err := i.pollUntilDone(ctx, async.ID)
	if err != nil {
		return nil, err
	}

	img, err := image.NewImage(final.Assets.Image, "")
	if err != nil {
		return nil, err
	}

	result, err := image.NewResult(img, &image.ResultMetadata{})
	if err != nil {
		return nil, err
	}

	meta := &image.ResponseMetadata{Created: time.Now().Unix()}
	if err := meta.Set("task_id", async.ID); err != nil {
		return nil, err
	}
	return image.NewResponse(result, meta)
}

func (i *ImageModel) pollUntilDone(ctx context.Context, id string) (*Generation, error) {
	deadline, cancel := context.WithTimeout(ctx, i.pollTimeout)
	defer cancel()
	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()
	for {
		resp, err := i.api.GetGeneration(deadline, id)
		if err != nil {
			return nil, err
		}
		switch resp.State {
		case "completed":
			return resp, nil
		case "failed":
			return nil, fmt.Errorf("luma: generation failed: %s", resp.FailureReason)
		}
		select {
		case <-deadline.Done():
			return nil, deadline.Err()
		case <-ticker.C:
		}
	}
}
