package luma

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	lumaagents "github.com/lumalabs/luma-agents-go"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client
	PollInterval   time.Duration
	PollTimeout    time.Duration
}

func (i ImageModelConfig) Validate() error {
	if i.APIKey == "" {
		return errors.New("luma: APIKey is required")
	}
	if i.DefaultOptions.Model == "" {
		return errors.New("luma: DefaultOptions.Model is required")
	}
	if _, err := i.DefaultOptions.Merged(); err != nil {
		return fmt.Errorf("luma: DefaultOptions: %w", err)
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel implements Luma Agents image generation and editing with the
// official SDK. Provider output URLs are downloaded before they expire.
type ImageModel struct {
	api            *api
	defaultOptions image.Options
	httpClient     *http.Client
	pollInterval   time.Duration
	pollTimeout    time.Duration
}

func NewImageModel(config ImageModelConfig) (*ImageModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(apiConfig{
		APIKey:     config.APIKey,
		BaseURL:    config.BaseURL,
		HTTPClient: config.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = time.Duration(DefaultPollIntervalSeconds) * time.Second
	}
	pollTimeout := config.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = time.Duration(DefaultPollTimeoutSeconds) * time.Second
	}
	return &ImageModel{
		api:            api,
		defaultOptions: config.DefaultOptions.Clone(),
		httpClient:     httpClient,
		pollInterval:   pollInterval,
		pollTimeout:    pollTimeout,
	}, nil
}

func (i *ImageModel) Call(ctx context.Context, request *image.Request) (*image.Response, error) {
	if i == nil || i.api == nil {
		return nil, errors.New("luma: nil ImageModel")
	}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("luma: request: %w", err)
	}
	optionsValue, err := i.defaultOptions.Merged(request.Options)
	if err != nil {
		return nil, err
	}
	if rejectUnsupportedOptionsErr := rejectUnsupportedOptions(optionsValue); rejectUnsupportedOptionsErr != nil {
		return nil, rejectUnsupportedOptionsErr
	}

	paramsValue, _, err := optionsValue.Extensions.Decode[lumaagents.GenerationNewParams](ImageRequestExtensionKey)
	if err != nil {
		return nil, fmt.Errorf("luma: extension %q: %w", ImageRequestExtensionKey, err)
	}
	params := &paramsValue
	params.Prompt = lumaagents.F(request.Prompt)
	params.Model = lumaagents.F(lumaagents.Model(optionsValue.Model))
	if !params.Type.Present {
		params.Type = lumaagents.F(lumaagents.GenerationNewParamsTypeImage)
	}
	if params.Type.Value != lumaagents.GenerationNewParamsTypeImage && params.Type.Value != lumaagents.GenerationNewParamsTypeImageEdit {
		return nil, fmt.Errorf("luma: extension %q type %q is not an image operation", ImageRequestExtensionKey, params.Type.Value)
	}
	if optionsValue.OutputFormat != "" {
		format := strings.TrimPrefix(optionsValue.OutputFormat, "image/")
		switch format {
		case "png":
			params.OutputFormat = lumaagents.F(lumaagents.GenerationNewParamsOutputFormatPng)
		case "jpeg", "jpg":
			params.OutputFormat = lumaagents.F(lumaagents.GenerationNewParamsOutputFormatJpeg)
		default:
			return nil, fmt.Errorf("luma: output format %q is unsupported; use image/png or image/jpeg", optionsValue.OutputFormat)
		}
	}

	submitted, err := i.api.createGeneration(ctx, *params)
	if err != nil {
		return nil, fmt.Errorf("luma: create generation: %w", err)
	}
	completed, err := i.pollUntilDone(ctx, submitted.ID)
	if err != nil {
		return nil, err
	}
	return i.mapResponse(ctx, completed)
}

func rejectUnsupportedOptions(options image.Options) error {
	unsupported := make([]string, 0, 4)
	if options.Height != nil {
		unsupported = append(unsupported, "height")
	}
	if options.NegativePrompt != "" {
		unsupported = append(unsupported, "negative_prompt")
	}
	if options.Seed != nil {
		unsupported = append(unsupported, "seed")
	}
	if options.Width != nil {
		unsupported = append(unsupported, "width")
	}
	if len(unsupported) == 0 {
		return nil
	}
	slices.Sort(unsupported)
	return fmt.Errorf("luma: image: unsupported options: %s", strings.Join(unsupported, ", "))
}

func (i *ImageModel) pollUntilDone(ctx context.Context, generationID string) (*lumaagents.Generation, error) {
	deadline, cancel := context.WithTimeout(ctx, i.pollTimeout)
	defer cancel()
	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()
	for {
		generation, err := i.api.getGeneration(deadline, generationID)
		if err != nil {
			return nil, fmt.Errorf("luma: get generation %q: %w", generationID, err)
		}
		switch generation.State {
		case lumaagents.GenerationStateCompleted:
			return generation, nil
		case lumaagents.GenerationStateFailed:
			return nil, fmt.Errorf("luma: generation %q failed (%s): %s", generationID, generation.FailureCode, generation.FailureReason)
		}
		select {
		case <-deadline.Done():
			return nil, deadline.Err()
		case <-ticker.C:
		}
	}
}

func (i *ImageModel) mapResponse(ctx context.Context, generation *lumaagents.Generation) (*image.Response, error) {
	if generation == nil {
		return nil, errors.New("luma: nil generation response")
	}
	if len(generation.Output) == 0 {
		return nil, fmt.Errorf("luma: generation %q completed without output", generation.ID)
	}
	outputs := make([]*image.Output, 0, len(generation.Output))
	for outputIndex := range generation.Output {
		generationOutput := generation.Output[outputIndex]
		data, mimeType, err := i.downloadOutput(ctx, generationOutput.URL)
		if err != nil {
			return nil, fmt.Errorf("luma: output[%d]: %w", outputIndex, err)
		}
		value, err := media.NewBytes(mimeType, data)
		if err != nil {
			return nil, err
		}
		outputMetadata := &image.OutputMetadata{}
		if setErr := outputMetadata.Set("luma/output", generationOutput); setErr != nil {
			return nil, setErr
		}
		output, err := image.NewOutput(value, outputMetadata)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, generation.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("luma: generation created_at %q: %w", generation.CreatedAt, err)
	}
	metadata := &image.ResponseMetadata{Created: createdAt.Unix()}
	if err := metadata.Set(ResponseExtensionKey, generation); err != nil {
		return nil, err
	}
	return image.NewResponse(outputs, metadata)
}

func (i *ImageModel) downloadOutput(ctx context.Context, rawURL string) ([]byte, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build download request: %w", err)
	}
	response, err := i.httpClient.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("download: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("download HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read download: %w", err)
	}
	if len(data) == 0 {
		return nil, "", errors.New("download is empty")
	}
	mimeType := strings.TrimSpace(strings.SplitN(response.Header.Get("Content-Type"), ";", 2)[0])
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	return data, mimeType, nil
}
