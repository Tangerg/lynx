package midjourney

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
	BaseURL        string // required: pick your proxy provider
	HTTPClient     *http.Client
	SubmitPath     string
	FetchPath      string
	AuthHeader     string
	AuthBearer     bool
	PollInterval   time.Duration
	PollTimeout    time.Duration
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("midjourney: APIKey is required")
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

// ImageModel wraps any Midjourney-compatible proxy (APIFrame /
// ImaginePro / TTAPI / GoAPI / UseAPI / ...). Configure the
// proxy-specific paths and auth scheme via [ImageModelConfig].
//
// IMPORTANT: lynx does not endorse any specific Midjourney proxy.
// Picking one is the caller's call — third-party proxies are not
// sanctioned by Midjourney and may violate their ToS. Account-ban risk
// is on the caller.
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
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if err := options.RejectUnsupported("midjourney: image", map[string]bool{
		"height":          mergedOpts.Height != nil,
		"negative_prompt": mergedOpts.NegativePrompt != "",
		"output_format":   mergedOpts.OutputFormat != "",
		"seed":            mergedOpts.Seed != nil,
		"width":           mergedOpts.Width != nil,
	}); err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[GenerateRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}
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

	imageURLs := pickImageURLs(final)
	if len(imageURLs) == 0 {
		return nil, errors.New("midjourney: fetch returned no image url")
	}

	results := make([]*image.Result, 0, len(imageURLs))
	for _, imageURL := range imageURLs {
		value, err := media.NewURI("application/octet-stream", imageURL)
		if err != nil {
			return nil, err
		}
		result, err := image.NewResult(value, &image.ResultMetadata{})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	meta := &image.ResponseMetadata{Created: time.Now().Unix()}
	if err := meta.Set("task_id", taskID); err != nil {
		return nil, err
	}
	return image.NewResponse(results, meta)
}

func pickImageURLs(r *FetchResponse) []string {
	if len(r.ImageURLs) > 0 {
		return r.ImageURLs
	}
	if r.ImageURL != "" {
		return []string{r.ImageURL}
	}
	if r.URI != "" {
		return []string{r.URI}
	}
	if r.Result != "" {
		return []string{r.Result}
	}
	return nil
}

func isTerminalSuccess(status string) bool {
	s := strings.ToUpper(status)
	return s == "SUCCESS" || s == "COMPLETED" || s == "DONE" || s == "FINISHED"
}

func isTerminalFailure(status string) bool {
	s := strings.ToUpper(status)
	return strings.Contains(s, "FAIL") || s == "ERROR" || s == "CANCELED"
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
