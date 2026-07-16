package google

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options

	// Backend / Project / Location enable Vertex AI access — see
	// the matching fields on [ChatConfig] for semantics.
	Backend  genai.Backend
	Project  string
	Location string

	// BaseURL overrides the genai endpoint. Optional.
	BaseURL string
}

func (c ImageModelConfig) Validate() error {
	if c.Backend != genai.BackendVertexAI && c.APIKey == "" {
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

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps Gemini's generate_images endpoint (Imagen 3 / Imagen 4).
//
// Lynx's W×H pixel knobs are intentionally NOT translated to Imagen's
// aspect-ratio code ("1:1" / "16:9" / ...): Imagen has no per-pixel
// size control, and inferring "1:1" from "1024×1024" would be a lossy
// guess. Callers needing precise aspect control set AspectRatio on the
// extension-threaded [genai.GenerateImagesConfig].
type ImageModel struct {
	api            *API
	defaultOptions image.Options
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

	return &ImageModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (i *ImageModel) buildAPIRequest(req *image.Request) (string, string, *genai.GenerateImagesConfig, error) {
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return "", "", nil, err
	}
	if err := options.RejectUnsupported("google: image", map[string]bool{
		"height": mergedOpts.Height != nil,
		"width":  mergedOpts.Width != nil,
	}); err != nil {
		return "", "", nil, err
	}

	cfg, err := options.GetParams[genai.GenerateImagesConfig](mergedOpts.Extensions, OptionsKey)
	if err != nil {
		return "", "", nil, err
	}

	if mergedOpts.NegativePrompt != "" {
		cfg.NegativePrompt = mergedOpts.NegativePrompt
	}
	if mergedOpts.Seed != nil {
		if *mergedOpts.Seed > int64(math.MaxInt32) {
			return "", "", nil, fmt.Errorf("google: image: seed exceeds int32: %d", *mergedOpts.Seed)
		}
		cfg.Seed = new(int32(*mergedOpts.Seed))
	}
	if mergedOpts.OutputFormat != "" {
		// Imagen accepts the full MIME type ("image/png" / "image/jpeg").
		cfg.OutputMIMEType = mergedOpts.OutputFormat
	}

	return mergedOpts.Model, req.Prompt, cfg, nil
}

func (i *ImageModel) buildResponse(apiResp *genai.GenerateImagesResponse) (*image.Response, error) {
	if len(apiResp.GeneratedImages) == 0 {
		return nil, errors.New("google: image response has no generated images")
	}

	results := make([]*image.Result, 0, len(apiResp.GeneratedImages))
	for index, generated := range apiResp.GeneratedImages {
		if generated.Image == nil {
			return nil, fmt.Errorf("google: generated image %d has no payload", index)
		}
		mimeType := generated.Image.MIMEType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		var value *media.Media
		var err error
		switch {
		case generated.Image.GCSURI != "" && len(generated.Image.ImageBytes) == 0:
			value, err = media.NewURI(mimeType, generated.Image.GCSURI)
		case generated.Image.GCSURI == "" && len(generated.Image.ImageBytes) > 0:
			value, err = media.NewBytes(mimeType, generated.Image.ImageBytes)
		default:
			return nil, fmt.Errorf("google: generated image %d has an ambiguous payload", index)
		}
		if err != nil {
			return nil, err
		}

		resultMetadata := &image.ResultMetadata{}
		if generated.EnhancedPrompt != "" {
			if err := resultMetadata.Set("enhanced_prompt", generated.EnhancedPrompt); err != nil {
				return nil, err
			}
		}
		if generated.RAIFilteredReason != "" {
			if err := resultMetadata.Set("rai_filtered_reason", generated.RAIFilteredReason); err != nil {
				return nil, err
			}
		}
		if generated.SafetyAttributes != nil {
			if err := resultMetadata.Set("safety_attributes", generated.SafetyAttributes); err != nil {
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
	if apiResp.PositivePromptSafetyAttributes != nil {
		if err := meta.Set("positive_prompt_safety_attributes", apiResp.PositivePromptSafetyAttributes); err != nil {
			return nil, err
		}
	}

	return image.NewResponse(results, meta)
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
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
