package google

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *image.Options

	// Backend / Project / Location enable Vertex AI access — see
	// the matching fields on [ChatModelConfig] for semantics.
	Backend  genai.Backend
	Project  string
	Location string

	// BaseURL overrides the genai endpoint. Optional.
	BaseURL string

	// Metadata overrides the [image.ModelMetadata] returned by [ImageModel.Metadata].
	// Zero Provider falls back to [Provider].
	Metadata *image.ModelMetadata
}

func (c ImageModelConfig) Validate() error {
	if c.Backend != genai.BackendVertexAI && c.APIKey == nil {
		return errors.New("google: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("google: DefaultOptions is required")
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps Gemini's generate_images endpoint (Imagen 3 / Imagen 4).
//
// Lynx's W×H pixel knobs are intentionally NOT translated to Imagen's
// aspect-ratio code ("1:1" / "16:9" / ...): Imagen has no per-pixel
// size control, and inferring "1:1" from "1024×1024" would be a lossy
// guess. Callers needing precise aspect control set AspectRatio on the
// Extra-threaded [genai.GenerateImagesConfig].
type ImageModel struct {
	api            *API
	defaultOptions *image.Options
	metadata       image.ModelMetadata
}

func NewImageModel(cfg ImageModelConfig) (*ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:   cfg.APIKey,
		Backend:  cfg.Backend,
		Project:  cfg.Project,
		Location: cfg.Location,
		BaseURL:  cfg.BaseURL,
	})
	if err != nil {
		return nil, err
	}

	info := image.ModelMetadata{Provider: Provider}
	if cfg.Metadata != nil {
		info = *cfg.Metadata
	}
	return &ImageModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		metadata:           info,
	}, nil
}

func (i *ImageModel) buildAPIRequest(req *image.Request) (string, string, *genai.GenerateImagesConfig, error) {
	mergedOpts, err := image.MergeOptions(i.defaultOptions, req.Options)
	if err != nil {
		return "", "", nil, err
	}

	cfg := options.GetParams[genai.GenerateImagesConfig](mergedOpts, OptionsKey)

	if mergedOpts.NegativePrompt != "" {
		cfg.NegativePrompt = mergedOpts.NegativePrompt
	}
	if mergedOpts.Seed != nil {
		cfg.Seed = new(int32(*mergedOpts.Seed))
	}
	if mergedOpts.OutputFormat != nil {
		// Imagen accepts the full MIME type ("image/png" / "image/jpeg").
		cfg.OutputMIMEType = mergedOpts.OutputFormat.TypeAndSubType()
	}

	return mergedOpts.Model, req.Prompt, cfg, nil
}

func (i *ImageModel) buildResponse(apiResp *genai.GenerateImagesResponse) (*image.Response, error) {
	// Imagen returns GeneratedImages sized by NumberOfImages (default 4
	// when unset). The image surface is single-result by design — see
	// image.Response — so we take the first generated image and ignore
	// extras. Callers needing N>1 should drop down to the genai SDK.
	if len(apiResp.GeneratedImages) == 0 {
		return nil, errors.New("google: image response has no generated images")
	}

	gen := apiResp.GeneratedImages[0]
	if gen.Image == nil {
		return nil, errors.New("google: first generated image has no payload")
	}

	// Imagen returns either GCS URIs (Vertex AI deployments) or inline
	// bytes (Gemini API). Map onto our two-shape Image: URL for hosted,
	// B64JSON for inline.
	var img *image.Image
	var err error
	switch {
	case gen.Image.GCSURI != "":
		img, err = image.NewImage(gen.Image.GCSURI, "")
	case len(gen.Image.ImageBytes) > 0:
		img, err = image.NewImage("", base64.StdEncoding.EncodeToString(gen.Image.ImageBytes))
	default:
		return nil, errors.New("google: first generated image has neither GCS URI nor bytes")
	}
	if err != nil {
		return nil, err
	}

	resultMeta := &image.ResultMetadata{}
	if gen.EnhancedPrompt != "" {
		resultMeta.Set("enhanced_prompt", gen.EnhancedPrompt)
	}
	if gen.RAIFilteredReason != "" {
		resultMeta.Set("rai_filtered_reason", gen.RAIFilteredReason)
	}
	if gen.SafetyAttributes != nil {
		resultMeta.Set("safety_attributes", gen.SafetyAttributes)
	}

	result, err := image.NewResult(img, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &image.ResponseMetadata{Created: time.Now().Unix()}
	if apiResp.PositivePromptSafetyAttributes != nil {
		meta.Set("positive_prompt_safety_attributes", apiResp.PositivePromptSafetyAttributes)
	}

	return image.NewResponse(result, meta)
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	modelName, prompt, cfg, err := i.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := i.api.Image(ctx, modelName, prompt, cfg)
	if err != nil {
		return nil, err
	}

	return i.buildResponse(apiResp)
}

func (i *ImageModel) DefaultOptions() image.Options {
	return *i.defaultOptions
}

func (i *ImageModel) Metadata() image.ModelMetadata {
	return i.metadata
}
