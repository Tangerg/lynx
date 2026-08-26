package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/models/google/internal/options"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("google: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("google: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

// ImageGenerationOptions carries the current Interactions API controls that
// do not have provider-neutral equivalents in [image.Options]. Store it under
// [ImageRequestExtensionKey].
type ImageGenerationOptions struct {
	AspectRatio           string                    `json:"aspect_ratio,omitempty"`
	ImageSize             string                    `json:"image_size,omitempty"`
	Delivery              string                    `json:"delivery,omitempty"`
	PreviousInteractionID string                    `json:"previous_interaction_id,omitempty"`
	Store                 *bool                     `json:"store,omitempty"`
	ThinkingLevel         string                    `json:"thinking_level,omitempty"`
	ThinkingSummaries     string                    `json:"thinking_summaries,omitempty"`
	ServiceTier           string                    `json:"service_tier,omitempty"`
	Labels                map[string]string         `json:"labels,omitempty"`
	InputImages           []*media.Media            `json:"input_images,omitempty"`
	GoogleSearch          *ImageGoogleSearchOptions `json:"google_search,omitempty"`
	SafetySettings        []ImageSafetySetting      `json:"safety_settings,omitempty"`
}

// ImageGoogleSearchOptions configures the image-generation guide's
// google_search tool. SearchTypes accepts "web_search" and "image_search".
type ImageGoogleSearchOptions struct {
	SearchTypes []string `json:"search_types,omitempty"`
}

// ImageSafetySetting mirrors the Interactions API safety-setting shape.
type ImageSafetySetting struct {
	Type      string `json:"type"`
	Threshold string `json:"threshold"`
	Method    string `json:"method,omitempty"`
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel uses the current Gemini Interactions API. Imagen's legacy
// GenerateImages endpoint is deliberately not exposed: Google has deprecated
// Imagen and scheduled it for shutdown on 2026-08-17.
type ImageModel struct {
	api            *api
	defaultOptions image.Options
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

	return &ImageModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

type imageInteractionRequest struct {
	Model                 string                            `json:"model"`
	Input                 any                               `json:"input"`
	Tools                 []imageInteractionTool            `json:"tools,omitempty"`
	ResponseFormat        imageInteractionResponseFormat    `json:"response_format"`
	Store                 *bool                             `json:"store,omitempty"`
	GenerationConfig      *imageInteractionGenerationConfig `json:"generation_config,omitempty"`
	PreviousInteractionID string                            `json:"previous_interaction_id,omitempty"`
	Labels                map[string]string                 `json:"labels,omitempty"`
	SafetySettings        []ImageSafetySetting              `json:"safety_settings,omitempty"`
	ServiceTier           string                            `json:"service_tier,omitempty"`
}

type imageInteractionContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     []byte `json:"data,omitempty"`
	URI      string `json:"uri,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type imageInteractionTool struct {
	Type        string   `json:"type"`
	SearchTypes []string `json:"search_types,omitempty"`
}

type imageInteractionResponseFormat struct {
	Type        string `json:"type"`
	MIMEType    string `json:"mime_type,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	ImageSize   string `json:"image_size,omitempty"`
	Delivery    string `json:"delivery,omitempty"`
}

type imageInteractionGenerationConfig struct {
	Seed              *int32 `json:"seed,omitempty"`
	ThinkingLevel     string `json:"thinking_level,omitempty"`
	ThinkingSummaries string `json:"thinking_summaries,omitempty"`
}

func (i *ImageModel) buildAPIRequest(req *image.Request) (*imageInteractionRequest, error) {
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if err := options.RejectUnsupported("google: image", map[string]bool{
		"height":          mergedOpts.Height != nil,
		"negative_prompt": mergedOpts.NegativePrompt != "",
		"width":           mergedOpts.Width != nil,
	}); err != nil {
		return nil, err
	}

	providerOptsValue, _, err := mergedOpts.Extensions.Decode[ImageGenerationOptions](ImageRequestExtensionKey)

	providerOpts := &providerOptsValue
	if err != nil {
		return nil, err
	}
	if err := validateImageGenerationOptions(mergedOpts.Model, providerOpts); err != nil {
		return nil, err
	}
	if mergedOpts.OutputFormat != "" && mergedOpts.OutputFormat != "image/png" && mergedOpts.OutputFormat != "image/jpeg" {
		return nil, fmt.Errorf("google: image: unsupported output format %q; use image/png or image/jpeg", mergedOpts.OutputFormat)
	}

	responseFormat := imageInteractionResponseFormat{
		Type:        "image",
		MIMEType:    mergedOpts.OutputFormat,
		AspectRatio: providerOpts.AspectRatio,
		ImageSize:   providerOpts.ImageSize,
		Delivery:    providerOpts.Delivery,
	}
	generationConfig := &imageInteractionGenerationConfig{
		ThinkingLevel:     providerOpts.ThinkingLevel,
		ThinkingSummaries: providerOpts.ThinkingSummaries,
	}
	if mergedOpts.Seed != nil {
		if *mergedOpts.Seed > int64(math.MaxInt32) {
			return nil, fmt.Errorf("google: image: seed exceeds int32: %d", *mergedOpts.Seed)
		}
		generationConfig.Seed = new(int32(*mergedOpts.Seed))
	}
	if generationConfig.Seed == nil && generationConfig.ThinkingLevel == "" && generationConfig.ThinkingSummaries == "" {
		generationConfig = nil
	}

	input := any(req.Prompt)
	if len(providerOpts.InputImages) > 0 {
		contents := make([]imageInteractionContent, 0, len(providerOpts.InputImages)+1)
		contents = append(contents, imageInteractionContent{Type: "text", Text: req.Prompt})
		for index, value := range providerOpts.InputImages {
			content, err := imageInteractionContentFromMedia(value)
			if err != nil {
				return nil, fmt.Errorf("google: image: input_images[%d]: %w", index, err)
			}
			contents = append(contents, content)
		}
		input = contents
	}

	apiReq := &imageInteractionRequest{
		Model:                 mergedOpts.Model,
		Input:                 input,
		ResponseFormat:        responseFormat,
		Store:                 providerOpts.Store,
		GenerationConfig:      generationConfig,
		PreviousInteractionID: providerOpts.PreviousInteractionID,
		Labels:                providerOpts.Labels,
		SafetySettings:        providerOpts.SafetySettings,
		ServiceTier:           providerOpts.ServiceTier,
	}
	if providerOpts.GoogleSearch != nil {
		apiReq.Tools = []imageInteractionTool{{
			Type:        "google_search",
			SearchTypes: slices.Clone(providerOpts.GoogleSearch.SearchTypes),
		}}
	}
	return apiReq, nil
}

func imageInteractionContentFromMedia(value *media.Media) (imageInteractionContent, error) {
	if err := value.Validate(); err != nil {
		return imageInteractionContent{}, err
	}
	mediaType, _, err := mime.ParseMediaType(value.MIME)
	if err != nil || !strings.HasPrefix(mediaType, "image/") {
		return imageInteractionContent{}, fmt.Errorf("MIME type %q is not an image", value.MIME)
	}
	if !slices.Contains([]string{
		"image/png", "image/jpeg", "image/webp", "image/heic", "image/heif", "image/gif", "image/bmp", "image/tiff",
	}, mediaType) {
		return imageInteractionContent{}, fmt.Errorf("MIME type %q is not supported by the Interactions API", mediaType)
	}
	content := imageInteractionContent{Type: "image", MIMEType: mediaType}
	switch value.Source.Kind {
	case media.SourceBytes:
		content.Data = slices.Clone(value.Source.Bytes)
	case media.SourceURI:
		content.URI = value.Source.URI
	default:
		return imageInteractionContent{}, fmt.Errorf("source kind %q is not supported by the Interactions API", value.Source.Kind)
	}
	return content, nil
}

func validateImageGenerationOptions(modelName string, opts *ImageGenerationOptions) error {
	if opts == nil {
		return errors.New("google: image: nil provider options")
	}
	if opts.AspectRatio != "" && !slices.Contains([]string{
		"1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9", "1:8", "8:1", "1:4", "4:1",
	}, opts.AspectRatio) {
		return fmt.Errorf("google: image: unsupported aspect ratio %q", opts.AspectRatio)
	}
	if opts.ImageSize != "" && !slices.Contains([]string{"512", "1K", "2K", "4K"}, opts.ImageSize) {
		return fmt.Errorf("google: image: unsupported image size %q", opts.ImageSize)
	}
	if opts.Delivery != "" && opts.Delivery != "inline" && opts.Delivery != "uri" {
		return fmt.Errorf("google: image: unsupported delivery %q", opts.Delivery)
	}
	if opts.ThinkingLevel != "" && !slices.Contains([]string{"minimal", "low", "medium", "high"}, opts.ThinkingLevel) {
		return fmt.Errorf("google: image: unsupported thinking level %q", opts.ThinkingLevel)
	}
	if opts.ThinkingSummaries != "" && opts.ThinkingSummaries != "auto" && opts.ThinkingSummaries != "none" {
		return fmt.Errorf("google: image: unsupported thinking summaries %q", opts.ThinkingSummaries)
	}
	if opts.ServiceTier != "" && !slices.Contains([]string{"flex", "standard", "priority"}, opts.ServiceTier) {
		return fmt.Errorf("google: image: unsupported service tier %q", opts.ServiceTier)
	}
	if opts.GoogleSearch != nil {
		for index, searchType := range opts.GoogleSearch.SearchTypes {
			if searchType != "web_search" && searchType != "image_search" {
				return fmt.Errorf("google: image: google_search.search_types[%d]: unsupported value %q", index, searchType)
			}
		}
	}
	switch modelName {
	case ModelGemini31FlashLiteImage:
		if opts.ImageSize != "" && opts.ImageSize != "1K" {
			return fmt.Errorf("google: image: model %q only supports image size 1K", modelName)
		}
		if opts.GoogleSearch != nil {
			return fmt.Errorf("google: image: model %q does not support Google Search grounding", modelName)
		}
		if opts.ThinkingLevel != "" && opts.ThinkingLevel != "minimal" && opts.ThinkingLevel != "high" {
			return fmt.Errorf("google: image: model %q only supports minimal or high thinking", modelName)
		}
	case ModelGemini25FlashImage:
		if opts.ImageSize != "" && opts.ImageSize != "1K" {
			return fmt.Errorf("google: image: model %q only supports image size 1K", modelName)
		}
	case ModelGemini3ProImage:
		if opts.ImageSize == "512" {
			return fmt.Errorf("google: image: model %q does not support image size 512", modelName)
		}
	}
	for index, setting := range opts.SafetySettings {
		if !slices.Contains([]string{
			"hate_speech", "dangerous_content", "harassment", "sexually_explicit",
			"image_hate", "image_dangerous_content", "image_harassment", "image_sexually_explicit", "jailbreak",
		}, setting.Type) {
			return fmt.Errorf("google: image: safety_settings[%d].type: unsupported value %q", index, setting.Type)
		}
		if !slices.Contains([]string{
			"block_low_and_above", "block_medium_and_above", "block_only_high", "block_none", "off",
		}, setting.Threshold) {
			return fmt.Errorf("google: image: safety_settings[%d].threshold: unsupported value %q", index, setting.Threshold)
		}
		if setting.Method != "" && setting.Method != "severity" && setting.Method != "probability" {
			return fmt.Errorf("google: image: safety_settings[%d].method: unsupported value %q", index, setting.Method)
		}
	}
	return nil
}

type imageInteractionResponse struct {
	ID      string            `json:"id"`
	Model   string            `json:"model"`
	Status  string            `json:"status"`
	Object  string            `json:"object"`
	Created string            `json:"created"`
	Updated string            `json:"updated"`
	Steps   []json.RawMessage `json:"steps"`
	Usage   json.RawMessage   `json:"usage"`
	Raw     json.RawMessage   `json:"-"`
}

type imageInteractionStep struct {
	Type    string                     `json:"type"`
	Content []json.RawMessage          `json:"content"`
	Error   *imageInteractionStepError `json:"error,omitempty"`
}

type imageInteractionStepError struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Details []json.RawMessage `json:"details,omitempty"`
}

type imageInteractionOutput struct {
	Type       string `json:"type"`
	Data       []byte `json:"data,omitempty"`
	URI        string `json:"uri,omitempty"`
	MIMEType   string `json:"mime_type,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

func (i *ImageModel) buildResponse(apiResp *imageInteractionResponse) (*image.Response, error) {
	if apiResp == nil {
		return nil, errors.New("google: image: nil Interactions response")
	}
	outputs := make([]*image.Output, 0)
	for stepIndex, rawStep := range apiResp.Steps {
		var step imageInteractionStep
		if err := json.Unmarshal(rawStep, &step); err != nil {
			return nil, fmt.Errorf("google: image: decode steps[%d]: %w", stepIndex, err)
		}
		if step.Error != nil {
			return nil, fmt.Errorf("google: image: steps[%d] failed with code %d: %s", stepIndex, step.Error.Code, step.Error.Message)
		}
		if step.Type != "model_output" {
			continue
		}
		for contentIndex, rawContent := range step.Content {
			var interactionOutput imageInteractionOutput
			if err := json.Unmarshal(rawContent, &interactionOutput); err != nil {
				return nil, fmt.Errorf("google: image: decode steps[%d].content[%d]: %w", stepIndex, contentIndex, err)
			}
			if interactionOutput.Type != "image" {
				continue
			}
			value, err := imageMediaFromInteractionOutput(interactionOutput)
			if err != nil {
				return nil, fmt.Errorf("google: image: steps[%d].content[%d]: %w", stepIndex, contentIndex, err)
			}
			outputMetadata := &image.OutputMetadata{}
			if err := outputMetadata.Set("google/image_content", rawContent); err != nil {
				return nil, err
			}
			output, err := image.NewOutput(value, outputMetadata)
			if err != nil {
				return nil, err
			}
			outputs = append(outputs, output)
		}
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("google: image: Interactions response status %q has no model-output images", apiResp.Status)
	}

	meta := &image.ResponseMetadata{}
	if apiResp.Created != "" {
		created, err := time.Parse(time.RFC3339, apiResp.Created)
		if err != nil {
			return nil, fmt.Errorf("google: image: invalid provider creation time %q: %w", apiResp.Created, err)
		}
		meta.Created = created.Unix()
	}
	if err := meta.Set(ImageResponseExtensionKey, apiResp.Raw); err != nil {
		return nil, err
	}
	return image.NewResponse(outputs, meta)
}

func imageMediaFromInteractionOutput(output imageInteractionOutput) (*media.Media, error) {
	if output.MIMEType == "" {
		return nil, errors.New("image output has no MIME type")
	}
	switch {
	case len(output.Data) > 0 && output.URI == "":
		return media.NewBytes(output.MIMEType, output.Data)
	case len(output.Data) == 0 && output.URI != "":
		return media.NewURI(output.MIMEType, output.URI)
	case len(output.Data) == 0:
		return nil, errors.New("image output has neither inline data nor URI")
	default:
		return nil, errors.New("image output has both inline data and URI")
	}
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := i.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := i.api.createImageInteraction(ctx, apiReq)
	if err != nil {
		return nil, err
	}
	return i.buildResponse(apiResp)
}
