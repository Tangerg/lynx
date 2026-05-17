package midjourney

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *image.Options
	BaseURL        string // required: pick your proxy provider
	HTTPClient     *http.Client
	SubmitPath     string
	FetchPath      string
	AuthHeader     string
	AuthBearer     bool
	PollInterval   time.Duration
	PollTimeout    time.Duration
}

func (c *ImageModelConfig) validate() error {
	if c == nil {
		return errors.New("midjourney: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("midjourney: ApiKey is required")
	}
	if c.BaseURL == "" {
		return errors.New("midjourney: BaseURL is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("midjourney: DefaultOptions is required")
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps any Midjourney-compatible proxy (ApiFrame /
// ImaginePro / TTAPI / GoAPI / UseAPI / ...). Configure the
// proxy-specific paths and auth scheme via [ImageModelConfig].
//
// IMPORTANT: lynx does not endorse any specific Midjourney proxy.
// Picking one is the caller's call — third-party proxies are not
// sanctioned by Midjourney and may violate their ToS. Account-ban risk
// is on the caller.
type ImageModel struct {
	api            *Api
	defaultOptions *image.Options
	pollInterval   time.Duration
	pollTimeout    time.Duration
}

func NewImageModel(cfg *ImageModelConfig) (*ImageModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	api, err := NewApi(&ApiConfig{
		ApiKey:     cfg.ApiKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
		SubmitPath: cfg.SubmitPath,
		FetchPath:  cfg.FetchPath,
		AuthHeader: cfg.AuthHeader,
		AuthBearer: cfg.AuthBearer,
	})
	if err != nil {
		return nil, err
	}
	pi := cfg.PollInterval
	if pi <= 0 {
		pi = DefaultPollInterval
	}
	pt := cfg.PollTimeout
	if pt <= 0 {
		pt = DefaultPollTimeout
	}
	return &ImageModel{api: api, defaultOptions: cfg.DefaultOptions, pollInterval: pi, pollTimeout: pt}, nil
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	mergedOpts, err := image.MergeOptions(i.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq := options.GetParams[GenerateRequest](mergedOpts, OptionsKey)
	if apiReq.Prompt == "" {
		apiReq.Prompt = req.Prompt
	}

	submitted, err := i.api.Submit(ctx, apiReq)
	if err != nil {
		return nil, err
	}
	taskID := submitted.ResolvedID()
	if taskID == "" {
		return nil, errors.New("midjourney: submit returned no task id")
	}

	final, err := i.pollUntilDone(ctx, taskID)
	if err != nil {
		return nil, err
	}

	imageURL := pickImageURL(final)
	if imageURL == "" {
		return nil, errors.New("midjourney: fetch returned no image url")
	}

	img, err := image.NewImage(imageURL, "")
	if err != nil {
		return nil, err
	}

	result, err := image.NewResult(img, &image.ResultMetadata{})
	if err != nil {
		return nil, err
	}

	meta := &image.ResponseMetadata{Created: time.Now().Unix()}
	meta.Set("task_id", taskID)
	return image.NewResponse(result, meta)
}

func pickImageURL(r *FetchResponse) string {
	if r.ImageURL != "" {
		return r.ImageURL
	}
	if len(r.ImageURLs) > 0 {
		return r.ImageURLs[0]
	}
	if r.URI != "" {
		return r.URI
	}
	return r.Result
}

func isTerminalSuccess(status string) bool {
	s := strings.ToUpper(status)
	return s == "SUCCESS" || s == "COMPLETED" || s == "DONE" || s == "FINISHED"
}

func isTerminalFailure(status string) bool {
	s := strings.ToUpper(status)
	return strings.Contains(s, "FAIL") || s == "ERROR" || s == "CANCELLED" || s == "CANCELED"
}

func (i *ImageModel) pollUntilDone(ctx context.Context, id string) (*FetchResponse, error) {
	deadline, cancel := context.WithTimeout(ctx, i.pollTimeout)
	defer cancel()
	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()
	for {
		resp, err := i.api.Fetch(deadline, id)
		if err != nil {
			return nil, err
		}
		if isTerminalSuccess(resp.Status) {
			return resp, nil
		}
		if isTerminalFailure(resp.Status) {
			return nil, fmt.Errorf("midjourney: generation failed: %s", resp.FailReason)
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
