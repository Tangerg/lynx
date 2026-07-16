package stability

import (
	"cmp"
	"context"
	"encoding/base64"
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
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client

	// Endpoint selects which v2beta engine to call. Defaults to
	// [EndpointCore] when empty. Use [EndpointUltra] for highest
	// quality or [EndpointSD3] for the Stable Diffusion 3 family
	// (which also requires Options.Extensions["stability/options"] to
	// pick the SD3 variant via the extension-threaded [GenerateRequest]).
	Endpoint string
}

func (c ImageModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("stability: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("stability: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
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
// extension-threaded [GenerateRequest] when control is needed.
//
// One ImageModel is locked to one engine via [ImageModelConfig.Endpoint]
// (Core / Ultra / SD3); callers wanting another tier construct another
// model.
type ImageModel struct {
	api            *API
	defaultOptions image.Options
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
		defaultOptions: cfg.DefaultOptions.Clone(),
		endpoint:       cmp.Or(cfg.Endpoint, EndpointCore),
	}, nil
}

func (i *ImageModel) buildAPIRequest(req *image.Request) (*GenerateRequest, error) {
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if err := options.RejectUnsupported("stability: image", map[string]bool{
		"height": mergedOpts.Height != nil,
		"width":  mergedOpts.Width != nil,
	}); err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[GenerateRequest](mergedOpts.Extensions, OptionsKey)
	if err != nil {
		return nil, err
	}

	apiReq.Prompt = req.Prompt
	if mergedOpts.NegativePrompt != "" {
		apiReq.NegativePrompt = mergedOpts.NegativePrompt
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

func (i *ImageModel) buildResponse(body []byte, hdr http.Header, outputFormat string) (*image.Response, error) {
	envelope, err := DecodeJSON(body)
	if err != nil {
		return nil, err
	}

	data, err := base64.StdEncoding.DecodeString(envelope.Image)
	if err != nil {
		return nil, fmt.Errorf("stability: decode image: %w", err)
	}
	mimeType := "image/png"
	if outputFormat != "" {
		mimeType = "image/" + outputFormat
	}
	value, err := media.NewBytes(mimeType, data)
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

	result, err := image.NewResult(value, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &image.ResponseMetadata{Created: time.Now().Unix()}
	if rid := hdr.Get("request-id"); rid != "" {
		if err := meta.Set("request_id", rid); err != nil {
			return nil, err
		}
	}

	return image.NewResponse([]*image.Result{result}, meta)
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

	return i.buildResponse(body, hdr, apiReq.OutputFormat)
}
