package vertexai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
	"strings"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/models/google/internal/options"
)

type ImageModelConfig struct {
	Project        string
	Location       string
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ImageModelConfig) Validate() error {
	if c.Project == "" {
		return errors.New("vertexai: Project is required")
	}
	if c.Location == "" {
		return errors.New("vertexai: Location is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("vertexai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

// ImageGenerationOptions carries Vertex-specific GenerateContent controls and
// optional source images for editing. Store it under
// [ImageRequestExtensionKey].
type ImageGenerationOptions struct {
	InputImages              []*media.Media `json:"input_images,omitempty"`
	AspectRatio              string         `json:"aspect_ratio,omitempty"`
	ImageSize                string         `json:"image_size,omitempty"`
	PersonGeneration         string         `json:"person_generation,omitempty"`
	ProminentPeople          string         `json:"prominent_people,omitempty"`
	OutputCompressionQuality *int32         `json:"output_compression_quality,omitempty"`
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel implements Gemini native image generation on Vertex AI through
// GenerateContent. It intentionally does not expose Imagen's Predict endpoint:
// Google deprecated the Imagen GA endpoints in favor of Gemini image models.
type ImageModel struct {
	client         *genai.Client
	defaultOptions image.Options
}

func NewImageModel(cfg ImageModelConfig) (*ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	clientConfig := &genai.ClientConfig{
		Backend:    genai.BackendVertexAI,
		Project:    cfg.Project,
		Location:   cfg.Location,
		HTTPClient: cfg.HTTPClient,
		HTTPOptions: genai.HTTPOptions{
			APIVersion: "v1",
			BaseURL:    cfg.BaseURL,
		},
	}
	client, err := genai.NewClient(context.Background(), clientConfig)
	if err != nil {
		return nil, fmt.Errorf("vertexai: create Gen AI client: %w", err)
	}
	return &ImageModel{client: client, defaultOptions: cfg.DefaultOptions.Clone()}, nil
}

func (m *ImageModel) buildRequest(req *image.Request) (string, []*genai.Content, *genai.GenerateContentConfig, error) {
	mergedOpts, err := m.defaultOptions.Merged(req.Options)
	if err != nil {
		return "", nil, nil, err
	}
	if err := options.RejectUnsupported("vertexai: image", map[string]bool{
		"height":          mergedOpts.Height != nil,
		"negative_prompt": mergedOpts.NegativePrompt != "",
		"width":           mergedOpts.Width != nil,
	}); err != nil {
		return "", nil, nil, err
	}

	providerOptsValue, _, err := mergedOpts.Extensions.Decode[ImageGenerationOptions](ImageRequestExtensionKey)

	providerOpts := &providerOptsValue
	if err != nil {
		return "", nil, nil, err
	}
	config := genai.GenerateContentConfig{
		ResponseModalities: []string{string(genai.ModalityImage)},
		ImageConfig: &genai.ImageConfig{
			AspectRatio:              providerOpts.AspectRatio,
			ImageSize:                providerOpts.ImageSize,
			PersonGeneration:         providerOpts.PersonGeneration,
			ProminentPeople:          genai.ProminentPeople(providerOpts.ProminentPeople),
			OutputCompressionQuality: providerOpts.OutputCompressionQuality,
		},
	}
	if mergedOpts.Seed != nil {
		if *mergedOpts.Seed > math.MaxInt32 {
			return "", nil, nil, fmt.Errorf("vertexai: image: seed exceeds int32: %d", *mergedOpts.Seed)
		}
		config.Seed = new(int32(*mergedOpts.Seed))
	}
	if mergedOpts.OutputFormat != "" {
		if mergedOpts.OutputFormat != "image/png" && mergedOpts.OutputFormat != "image/jpeg" {
			return "", nil, nil, fmt.Errorf("vertexai: image: unsupported output format %q; use image/png or image/jpeg", mergedOpts.OutputFormat)
		}
		if config.ImageConfig == nil {
			config.ImageConfig = &genai.ImageConfig{}
		}
		config.ImageConfig.OutputMIMEType = mergedOpts.OutputFormat
	}

	parts := make([]*genai.Part, 0, len(providerOpts.InputImages)+1)
	parts = append(parts, genai.NewPartFromText(req.Prompt))
	for index, value := range providerOpts.InputImages {
		part, err := vertexImagePart(value)
		if err != nil {
			return "", nil, nil, fmt.Errorf("vertexai: image: input_images[%d]: %w", index, err)
		}
		parts = append(parts, part)
	}
	contents := []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}
	return mergedOpts.Model, contents, &config, nil
}

func vertexImagePart(value *media.Media) (*genai.Part, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	mediaType, _, err := mime.ParseMediaType(value.MIME)
	if err != nil || !strings.HasPrefix(mediaType, "image/") {
		return nil, fmt.Errorf("MIME type %q is not an image", value.MIME)
	}
	switch value.Source.Kind {
	case media.SourceBytes:
		return genai.NewPartFromBytes(value.Source.Bytes, mediaType), nil
	case media.SourceURI:
		return genai.NewPartFromURI(value.Source.URI, mediaType), nil
	default:
		return nil, fmt.Errorf("source kind %q is not supported by Vertex GenerateContent", value.Source.Kind)
	}
}

func (m *ImageModel) buildResponse(apiResp *genai.GenerateContentResponse) (*image.Response, error) {
	if apiResp == nil {
		return nil, errors.New("vertexai: image: nil GenerateContent response")
	}
	outputs := make([]*image.Output, 0)
	for candidateIndex, candidate := range apiResp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for partIndex, part := range candidate.Content.Parts {
			if part == nil || part.Thought {
				continue
			}
			var value *media.Media
			var err error
			switch {
			case part.InlineData != nil && strings.HasPrefix(part.InlineData.MIMEType, "image/"):
				value, err = media.NewBytes(part.InlineData.MIMEType, part.InlineData.Data)
			case part.FileData != nil && strings.HasPrefix(part.FileData.MIMEType, "image/"):
				value, err = media.NewURI(part.FileData.MIMEType, part.FileData.FileURI)
			default:
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("vertexai: image: candidates[%d].parts[%d]: %w", candidateIndex, partIndex, err)
			}
			outputMetadata := &image.OutputMetadata{}
			if err := outputMetadata.Set("vertexai/native_part", part); err != nil {
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
		return nil, errors.New("vertexai: image: GenerateContent response has no final image parts")
	}

	metadata := &image.ResponseMetadata{}
	if !apiResp.CreateTime.IsZero() {
		metadata.Created = apiResp.CreateTime.Unix()
	}
	if err := metadata.Set(ImageResponseExtensionKey, apiResp); err != nil {
		return nil, err
	}
	return image.NewResponse(outputs, metadata)
}

func (m *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if m == nil || m.client == nil {
		return nil, errors.New("vertexai: image: nil model")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	modelName, contents, config, err := m.buildRequest(req)
	if err != nil {
		return nil, err
	}
	apiResp, err := m.client.Models.GenerateContent(ctx, modelName, contents, config)
	if err != nil {
		return nil, fmt.Errorf("vertexai: image: GenerateContent: %w", err)
	}
	return m.buildResponse(apiResp)
}
