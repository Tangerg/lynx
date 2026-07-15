package replicate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions *image.Options
	BaseURL        string
	HTTPClient     *http.Client

	// PollInterval / PollTimeout configure the synchronous wrapper
	// around Replicate's async generation. Zero values fall back to
	// the package defaults.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
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
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	apiReq, err := options.GetParams[PredictionRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}
	if apiReq.Input == nil {
		apiReq.Input = map[string]any{}
	}
	apiReq.Input["prompt"] = req.Prompt
	if mergedOpts.NegativePrompt != "" {
		apiReq.Input["negative_prompt"] = mergedOpts.NegativePrompt
	}
	if mergedOpts.Width != nil {
		width, err := options.Int("replicate: image: width", *mergedOpts.Width)
		if err != nil {
			return nil, err
		}
		apiReq.Input["width"] = width
	}
	if mergedOpts.Height != nil {
		height, err := options.Int("replicate: image: height", *mergedOpts.Height)
		if err != nil {
			return nil, err
		}
		apiReq.Input["height"] = height
	}
	if mergedOpts.Seed != nil {
		apiReq.Input["seed"] = *mergedOpts.Seed
	}
	if mergedOpts.OutputFormat != "" {
		if _, set := apiReq.Input["output_format"]; !set {
			apiReq.Input["output_format"] = strings.TrimPrefix(mergedOpts.OutputFormat, "image/")
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

	urls, err := imageURLs(final.Output)
	if err != nil {
		return nil, err
	}

	mimeType := mergedOpts.OutputFormat
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	results := make([]*image.Result, 0, len(urls))
	for _, url := range urls {
		value, err := media.NewURI(mimeType, url)
		if err != nil {
			return nil, err
		}
		resultMetadata := &image.ResultMetadata{}
		if final.Metrics.PredictTime > 0 {
			if err := resultMetadata.Set("predict_time_ms", int64(final.Metrics.PredictTime*1000)); err != nil {
				return nil, err
			}
		}
		result, err := image.NewResult(value, resultMetadata)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	meta := &image.ResponseMetadata{Created: time.Now().Unix()}
	if err := meta.Set("model", mergedOpts.Model); err != nil {
		return nil, err
	}
	if err := meta.Set("prediction_id", final.ID); err != nil {
		return nil, err
	}
	if final.Version != "" {
		if err := meta.Set("version", final.Version); err != nil {
			return nil, err
		}
	}
	return image.NewResponse(results, meta)
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

// imageURLs extracts every hosted image URL from a Replicate prediction.
func imageURLs(out any) ([]string, error) {
	switch v := out.(type) {
	case string:
		if v == "" {
			return nil, errors.New("replicate: empty output URL")
		}
		return []string{v}, nil
	case []any:
		if len(v) == 0 {
			return nil, errors.New("replicate: empty output array")
		}
		urls := make([]string, len(v))
		for index, value := range v {
			url, ok := value.(string)
			if !ok || url == "" {
				return nil, fmt.Errorf("replicate: invalid output element %d of type %T", index, value)
			}
			urls[index] = url
		}
		return urls, nil
	case nil:
		return nil, errors.New("replicate: output is null")
	default:
		return nil, fmt.Errorf("replicate: unsupported output type %T", out)
	}
}
