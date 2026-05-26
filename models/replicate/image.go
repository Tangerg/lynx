package replicate

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
	// around Replicate's async generation. Zero values fall back to
	// the package defaults.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func (c *ImageModelConfig) validate() error {
	if c == nil {
		return errors.New("replicate: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("replicate: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("replicate: DefaultOptions is required")
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps Replicate's image-generation surface. The model
// id (set on [image.Options].Model) picks the upstream model — any
// image model on replicate.com that accepts a "prompt" input field
// works; lynx maps Width / Height / Seed / NegativePrompt /
// OutputFormat onto the canonical input keys and leaves the rest of
// the model-specific schema to Extra-threaded params.
type ImageModel struct {
	api            *API
	defaultOptions *image.Options
	pollInterval   time.Duration
	pollTimeout    time.Duration
}

func NewImageModel(cfg *ImageModelConfig) (*ImageModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(&APIConfig{
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

	apiReq := options.GetParams[PredictionRequest](mergedOpts, OptionsKey)
	if apiReq.Input == nil {
		apiReq.Input = map[string]any{}
	}
	apiReq.Input["prompt"] = req.Prompt
	if mergedOpts.NegativePrompt != "" {
		apiReq.Input["negative_prompt"] = mergedOpts.NegativePrompt
	}
	if mergedOpts.Width != nil {
		apiReq.Input["width"] = int(*mergedOpts.Width)
	}
	if mergedOpts.Height != nil {
		apiReq.Input["height"] = int(*mergedOpts.Height)
	}
	if mergedOpts.Seed != nil {
		apiReq.Input["seed"] = *mergedOpts.Seed
	}
	if mergedOpts.OutputFormat != nil {
		if _, set := apiReq.Input["output_format"]; !set {
			apiReq.Input["output_format"] = mergedOpts.OutputFormat.SubType()
		}
	}

	submit, err := i.api.CreatePrediction(ctx, mergedOpts.Model, apiReq)
	if err != nil {
		return nil, err
	}

	final, err := i.pollUntilDone(ctx, submit.ID)
	if err != nil {
		return nil, err
	}

	url, err := firstImageURL(final.Output)
	if err != nil {
		return nil, err
	}

	img, err := image.NewImage(url, "")
	if err != nil {
		return nil, err
	}

	resultMeta := &image.ResultMetadata{}
	if final.Metrics.PredictTime > 0 {
		resultMeta.Set("predict_time_ms", int64(final.Metrics.PredictTime*1000))
	}

	result, err := image.NewResult(img, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &image.ResponseMetadata{Created: time.Now().Unix()}
	meta.Set("model", mergedOpts.Model)
	meta.Set("prediction_id", final.ID)
	if final.Version != "" {
		meta.Set("version", final.Version)
	}
	return image.NewResponse(result, meta)
}

// pollUntilDone blocks until the prediction reaches a terminal status.
func (i *ImageModel) pollUntilDone(ctx context.Context, id string) (*PredictionResponse, error) {
	deadline, cancel := context.WithTimeout(ctx, i.pollTimeout)
	defer cancel()

	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()

	for {
		resp, err := i.api.GetPrediction(deadline, id)
		if err != nil {
			return nil, err
		}
		switch resp.Status {
		case "succeeded":
			return resp, nil
		case "failed", "canceled":
			msg := resp.Error
			if msg == "" {
				msg = resp.Status
			}
			return nil, fmt.Errorf("replicate: generation %s: %s", resp.Status, msg)
		}
		select {
		case <-deadline.Done():
			return nil, deadline.Err()
		case <-ticker.C:
		}
	}
}

// firstImageURL extracts the first hosted image URL from a Replicate
// prediction output. The shape varies per model — most image models
// return []string, some return a single string.
func firstImageURL(out any) (string, error) {
	switch v := out.(type) {
	case string:
		if v == "" {
			return "", errors.New("replicate: empty output URL")
		}
		return v, nil
	case []any:
		if len(v) == 0 {
			return "", errors.New("replicate: empty output array")
		}
		s, ok := v[0].(string)
		if !ok {
			return "", fmt.Errorf("replicate: unsupported output element type %T", v[0])
		}
		return s, nil
	case nil:
		return "", errors.New("replicate: output is null")
	default:
		return "", fmt.Errorf("replicate: unsupported output type %T", out)
	}
}

func (i *ImageModel) DefaultOptions() image.Options { return *i.defaultOptions }
func (i *ImageModel) Metadata() image.ModelMetadata         { return image.ModelMetadata{Provider: Provider} }
