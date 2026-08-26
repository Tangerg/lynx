package blackforestlabs

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client

	// PollInterval / PollTimeout configure the synchronous wrapper
	// around BFL's async generation. Zero values fall back to the
	// package defaults.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("blackforestlabs: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("blackforestlabs: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps Black Forest Labs' Flux image-generation endpoints.
// Model id picks the engine ("flux-pro-1.1", "flux-pro-1.1-ultra",
// "flux-dev", "flux-kontext-pro", "flux-kontext-max"). BFL is async
// only — Call submits + polls until ready.
type ImageModel struct {
	api            *api
	defaultOptions image.Options
	pollInterval   time.Duration
	pollTimeout    time.Duration
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
	pi := cfg.PollInterval
	if pi <= 0 {
		pi = time.Duration(DefaultPollIntervalSeconds) * time.Second
	}
	pt := cfg.PollTimeout
	if pt <= 0 {
		pt = time.Duration(DefaultPollTimeoutSeconds) * time.Second
	}
	return &ImageModel{api: api, defaultOptions: cfg.DefaultOptions.Clone(), pollInterval: pi, pollTimeout: pt}, nil
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if err := options.RejectUnsupported("blackforestlabs: image", map[string]bool{
		"negative_prompt": mergedOpts.NegativePrompt != "",
	}); err != nil {
		return nil, err
	}

	apiReqValue, _, err := mergedOpts.Extensions.Decode[generateRequest](ImageRequestExtensionKey)

	apiReq := &apiReqValue
	if err != nil {
		return nil, err
	}
	apiReq.Prompt = req.Prompt
	if mergedOpts.Width != nil {
		apiReq.Width, err = options.Int("blackforestlabs: image: width", *mergedOpts.Width)
		if err != nil {
			return nil, err
		}
	}
	if mergedOpts.Height != nil {
		apiReq.Height, err = options.Int("blackforestlabs: image: height", *mergedOpts.Height)
		if err != nil {
			return nil, err
		}
	}
	if mergedOpts.Seed != nil {
		apiReq.Seed = mergedOpts.Seed
	}
	if mergedOpts.OutputFormat != "" && apiReq.OutputFormat == "" {
		apiReq.OutputFormat = strings.TrimPrefix(mergedOpts.OutputFormat, "image/")
	}

	async, err := i.api.generate(ctx, mergedOpts.Model, apiReq)
	if err != nil {
		return nil, err
	}

	if async.ID == "" {
		return nil, errors.New("blackforestlabs: generation response has no task id")
	}
	if async.PollingURL == "" {
		return nil, errors.New("blackforestlabs: generation response has no polling_url")
	}

	final, err := i.pollUntilDone(ctx, async.PollingURL)
	if err != nil {
		return nil, err
	}
	if final.Result.Sample == "" {
		return nil, errors.New("blackforestlabs: ready output has no sample URL")
	}

	data, mimeType, err := i.api.downloadOutput(ctx, final.Result.Sample)
	if err != nil {
		return nil, err
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension("." + apiReq.OutputFormat)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	value, err := media.NewBytes(mimeType, data)
	if err != nil {
		return nil, err
	}

	outputMetadata := &image.OutputMetadata{}
	if err := outputMetadata.Set("blackforestlabs/output_url", final.Result.Sample); err != nil {
		return nil, err
	}
	if final.Result.Seed != 0 {
		if err := outputMetadata.Set("blackforestlabs/seed", final.Result.Seed); err != nil {
			return nil, err
		}
	}
	if final.Result.Duration != 0 {
		if err := outputMetadata.Set("blackforestlabs/duration_ms", final.Result.Duration); err != nil {
			return nil, err
		}
	}

	output, err := image.NewOutput(value, outputMetadata)
	if err != nil {
		return nil, err
	}

	meta := &image.ResponseMetadata{}
	if err := meta.Set("blackforestlabs/task_id", async.ID); err != nil {
		return nil, err
	}
	if err := meta.Set("blackforestlabs/submit_response", async); err != nil {
		return nil, err
	}
	if err := meta.Set("blackforestlabs/result_response", final); err != nil {
		return nil, err
	}
	return image.NewResponse([]*image.Output{output}, meta)
}

func (i *ImageModel) pollUntilDone(ctx context.Context, pollingURL string) (*pollResult, error) {
	deadline, cancel := context.WithTimeout(ctx, i.pollTimeout)
	defer cancel()

	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()

	for {
		resp, err := i.api.getResult(deadline, pollingURL)
		if err != nil {
			return nil, err
		}
		switch resp.Status {
		case "Ready":
			return resp, nil
		case "Error", "Failed", "Content Moderated", "Request Moderated", "Task not found":
			return nil, fmt.Errorf("blackforestlabs: generation failed: %s", resp.Status)
		}
		select {
		case <-deadline.Done():
			return nil, deadline.Err()
		case <-ticker.C:
		}
	}
}
