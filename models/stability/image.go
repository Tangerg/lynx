package stability

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions *image.Options
	BaseURL        string
	HTTPClient     *http.Client

	// Endpoint selects which v2beta engine to call. Defaults to
	// [EndpointCore] when empty. Use [EndpointUltra] for highest
	// quality or [EndpointSD3] for the Stable Diffusion 3 family
	// (which also requires Options.Extra["model"] to pick the SD3
	// variant via the Extra-threaded [GenerateRequest]).
	Endpoint string
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("stability: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("stability: DefaultOptions is required")
	}
	return nil
}

var _ image.Model = (*ImageModel)(nil)

// ImageModel wraps Stability AI's Stable Image / SD3 endpoints.
//
// Stability uses an aspect-ratio code ("1:1" / "16:9" / ...) rather
// than per-pixel W×H sizes — Core/Ultra render at a fixed total pixel
// budget. Lynx's Width/Height options are intentionally NOT translated
// to an aspect ratio (lossy guess); set AspectRatio on the
// Extra-threaded [GenerateRequest] when control is needed.
//
// One ImageModel is locked to one engine via [ImageModelConfig.Endpoint]
// (Core / Ultra / SD3); callers wanting another tier construct another
// model.
type ImageModel struct {
	api            *API
	defaultOptions *image.Options
	endpoint       string
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

	return &ImageModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		endpoint:       cmp.Or(cfg.Endpoint, EndpointCore),
	}, nil
}

func (i *ImageModel) buildAPIRequest(req *image.Request) (*GenerateRequest, error) {
	mergedOpts, err := image.MergeOptions(i.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[GenerateRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}

	apiReq.Prompt = req.Prompt
	if mergedOpts.NegativePrompt != "" {
		apiReq.NegativePrompt = mergedOpts.NegativePrompt
	}
	if mergedOpts.Style != "" && apiReq.StylePreset == "" {
		apiReq.StylePreset = mergedOpts.Style
	}
	if mergedOpts.Seed != nil {
		apiReq.Seed = mergedOpts.Seed
	}
	if mergedOpts.OutputFormat != "" && apiReq.OutputFormat == "" {
		apiReq.OutputFormat = strings.TrimPrefix(mergedOpts.OutputFormat, "image/")
	}

	// Force JSON mode to get FinishReason / Seed echoed back.
	apiReq.Mode = ResponseModeJSON

	return apiReq, nil
}

func (i *ImageModel) buildResponse(body []byte, hdr http.Header) (*image.Response, error) {
	envelope, err := DecodeJSON(body)
	if err != nil {
		return nil, err
	}

	img, err := image.NewImage("", envelope.Image)
	if err != nil {
		return nil, err
	}

	resultMeta := &image.ResultMetadata{}
	if envelope.FinishReason != "" {
		if err := resultMeta.Set("finish_reason", envelope.FinishReason); err != nil {
			return nil, err
		}
	}
	if envelope.Seed != 0 {
		if err := resultMeta.Set("seed", envelope.Seed); err != nil {
			return nil, err
		}
	}

	result, err := image.NewResult(img, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &image.ResponseMetadata{Created: time.Now().Unix()}
	if rid := hdr.Get("request-id"); rid != "" {
		if err := meta.Set("request_id", rid); err != nil {
			return nil, err
		}
	}

	return image.NewResponse(result, meta)
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := i.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	body, hdr, err := i.api.Generate(ctx, i.endpoint, apiReq)
	if err != nil {
		return nil, err
	}

	return i.buildResponse(body, hdr)
}
