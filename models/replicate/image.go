package replicate

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	InputSchema    ImageInputSchema
	BaseURL        string
	HTTPClient     *http.Client

	// PollInterval / PollTimeout configure the synchronous wrapper
	// around Replicate's async generation. Zero values fall back to
	// the package defaults.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func (i ImageModelConfig) Validate() error {
	if i.APIKey == "" {
		return errors.New("replicate: APIKey is required")
	}
	if i.DefaultOptions.Model == "" {
		return errors.New("replicate: DefaultOptions.Model is required")
	}
	if _, err := i.DefaultOptions.Merged(); err != nil {
		return err
	}
	if err := i.InputSchema.Validate(); err != nil {
		return err
	}
	return nil
}

// FileOutputKind identifies the exact file-output schema declared by a
// Replicate model version.
type FileOutputKind string

const (
	FileOutputURI     FileOutputKind = "uri"
	FileOutputURIList FileOutputKind = "uri_list"
)

// ImageInputSchema explicitly binds provider-neutral image fields to one
// Replicate model's OpenAPI input/output schema. Empty optional keys mean the
// model cannot represent that Core option; setting the option then fails
// instead of guessing a similarly named provider field.
type ImageInputSchema struct {
	PromptKey         string
	NegativePromptKey string
	WidthKey          string
	HeightKey         string
	SeedKey           string
	OutputFormatKey   string
	OutputFormats     map[string]string
	OutputKind        FileOutputKind
}

func (i ImageInputSchema) Clone() ImageInputSchema {
	i.OutputFormats = maps.Clone(i.OutputFormats)
	return i
}

func (i ImageInputSchema) Validate() error {
	if i.PromptKey == "" {
		return errors.New("replicate: ImageInputSchema.PromptKey is required")
	}
	if i.OutputKind != FileOutputURI && i.OutputKind != FileOutputURIList {
		return fmt.Errorf("replicate: ImageInputSchema.OutputKind must be %q or %q", FileOutputURI, FileOutputURIList)
	}
	if i.OutputFormatKey == "" && len(i.OutputFormats) > 0 {
		return errors.New("replicate: ImageInputSchema.OutputFormats requires OutputFormatKey")
	}
	if i.OutputFormatKey != "" && len(i.OutputFormats) == 0 {
		return errors.New("replicate: ImageInputSchema.OutputFormatKey requires OutputFormats")
	}
	for mimeType, value := range i.OutputFormats {
		if !strings.HasPrefix(mimeType, "image/") || value == "" {
			return fmt.Errorf("replicate: ImageInputSchema.OutputFormats contains invalid mapping %q -> %q", mimeType, value)
		}
	}
	keys := []string{i.PromptKey, i.NegativePromptKey, i.WidthKey, i.HeightKey, i.SeedKey, i.OutputFormatKey}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("replicate: ImageInputSchema maps multiple fields to input key %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (i ImageInputSchema) validateOptions(options image.Options) error {
	unsupported := make([]string, 0, 5)
	if options.Height != nil && i.HeightKey == "" {
		unsupported = append(unsupported, "height")
	}
	if options.NegativePrompt != "" && i.NegativePromptKey == "" {
		unsupported = append(unsupported, "negative_prompt")
	}
	if options.OutputFormat != "" && i.OutputFormatKey == "" {
		unsupported = append(unsupported, "output_format")
	}
	if options.Seed != nil && i.SeedKey == "" {
		unsupported = append(unsupported, "seed")
	}
	if options.Width != nil && i.WidthKey == "" {
		unsupported = append(unsupported, "width")
	}
	if len(unsupported) == 0 {
		return nil
	}
	return fmt.Errorf("replicate: image: unsupported options: %s", strings.Join(unsupported, ", "))
}

// FluxSchnellImageInputSchema returns the current official schema binding for
// black-forest-labs/flux-schnell. Dimensions, seed, and negative prompt are
// intentionally absent because that model's current schema does not expose
// them.
func FluxSchnellImageInputSchema() ImageInputSchema {
	return ImageInputSchema{
		PromptKey:       "prompt",
		OutputFormatKey: "output_format",
		OutputFormats: map[string]string{
			"image/jpeg": "jpg",
			"image/png":  "png",
			"image/webp": "webp",
		},
		OutputKind: FileOutputURIList,
	}
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps one explicitly bound Replicate image-model schema. A model
// override is rejected because Replicate models do not share an input or
// output contract; construct another adapter with the matching schema instead.
type ImageModel struct {
	api            *api
	model          string
	inputSchema    ImageInputSchema
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
	return &ImageModel{
		api:            api,
		model:          cfg.DefaultOptions.Model,
		inputSchema:    cfg.InputSchema.Clone(),
		defaultOptions: cfg.DefaultOptions.Clone(),
		pollInterval:   pi,
		pollTimeout:    pt,
	}, nil
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if mergedOpts.Model != i.model {
		return nil, fmt.Errorf("replicate: image: model override %q does not match bound schema for %q", mergedOpts.Model, i.model)
	}
	if err := i.inputSchema.validateOptions(mergedOpts); err != nil {
		return nil, err
	}
	apiReqValue, _, err := mergedOpts.Extensions.Decode[predictionRequest](ImageRequestExtensionKey)
	apiReq := &apiReqValue
	if err != nil {
		return nil, err
	}
	if apiReq.Input == nil {
		apiReq.Input = map[string]any{}
	}
	apiReq.Input[i.inputSchema.PromptKey] = req.Prompt
	if mergedOpts.NegativePrompt != "" {
		apiReq.Input[i.inputSchema.NegativePromptKey] = mergedOpts.NegativePrompt
	}
	if mergedOpts.Width != nil {
		apiReq.Input[i.inputSchema.WidthKey] = *mergedOpts.Width
	}
	if mergedOpts.Height != nil {
		apiReq.Input[i.inputSchema.HeightKey] = *mergedOpts.Height
	}
	if mergedOpts.Seed != nil {
		apiReq.Input[i.inputSchema.SeedKey] = *mergedOpts.Seed
	}
	if mergedOpts.OutputFormat != "" {
		value, supported := i.inputSchema.OutputFormats[mergedOpts.OutputFormat]
		if !supported {
			return nil, fmt.Errorf("replicate: image: model %q does not support output format %q", i.model, mergedOpts.OutputFormat)
		}
		apiReq.Input[i.inputSchema.OutputFormatKey] = value
	}

	submit, err := i.api.createPrediction(ctx, mergedOpts.Model, apiReq)
	if err != nil {
		return nil, err
	}

	final, err := i.pollUntilDone(ctx, submit.ID)
	if err != nil {
		return nil, err
	}

	urls, err := imageURLs(final.Output, i.inputSchema.OutputKind)
	if err != nil {
		return nil, err
	}

	outputs := make([]*image.Output, 0, len(urls))
	for outputIndex, outputURL := range urls {
		data, contentType, err := i.api.downloadOutput(ctx, outputURL)
		if err != nil {
			return nil, fmt.Errorf("replicate: image output[%d]: %w", outputIndex, err)
		}
		mimeType := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
		if mimeType == "" {
			mimeType = mergedOpts.OutputFormat
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		value, err := media.NewBytes(mimeType, data)
		if err != nil {
			return nil, err
		}
		outputMetadata := &image.OutputMetadata{}
		if err := outputMetadata.Set("replicate/output_url", outputURL); err != nil {
			return nil, err
		}
		if final.Metrics.PredictTime > 0 {
			if err := outputMetadata.Set("replicate/predict_time_ms", int64(final.Metrics.PredictTime*1000)); err != nil {
				return nil, err
			}
		}
		output, err := image.NewOutput(value, outputMetadata)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}

	meta := &image.ResponseMetadata{}
	if final.CreatedAt != "" {
		createdAt, err := time.Parse(time.RFC3339Nano, final.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("replicate: invalid prediction created_at %q: %w", final.CreatedAt, err)
		}
		meta.Created = createdAt.Unix()
	}
	if err := meta.Set("replicate/model", mergedOpts.Model); err != nil {
		return nil, err
	}
	if err := meta.Set("replicate/prediction_id", final.ID); err != nil {
		return nil, err
	}
	if final.Version != "" {
		if err := meta.Set("replicate/version", final.Version); err != nil {
			return nil, err
		}
	}
	if err := meta.Set(ImageResponseExtensionKey, final); err != nil {
		return nil, err
	}
	return image.NewResponse(outputs, meta)
}

// pollUntilDone blocks until the prediction reaches a terminal status.
func (i *ImageModel) pollUntilDone(ctx context.Context, id string) (*predictionResponse, error) {
	deadline, cancel := context.WithTimeout(ctx, i.pollTimeout)
	defer cancel()

	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()

	for {
		resp, err := i.api.getPrediction(deadline, id)
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
func imageURLs(out any, kind FileOutputKind) ([]string, error) {
	if out == nil {
		return nil, errors.New("replicate: image output is null")
	}
	switch kind {
	case FileOutputURI:
		value, ok := out.(string)
		if !ok || value == "" {
			return nil, fmt.Errorf("replicate: image output must be a non-empty URI, got %T", out)
		}
		return []string{value}, nil
	case FileOutputURIList:
		values, ok := out.([]any)
		if !ok || len(values) == 0 {
			return nil, fmt.Errorf("replicate: image output must be a non-empty URI array, got %T", out)
		}
		urls := make([]string, len(values))
		for index, value := range values {
			url, ok := value.(string)
			if !ok || url == "" {
				return nil, fmt.Errorf("replicate: image output[%d] must be a non-empty URI, got %T", index, value)
			}
			urls[index] = url
		}
		return urls, nil
	default:
		return nil, fmt.Errorf("replicate: unsupported image output schema %q", kind)
	}
}
