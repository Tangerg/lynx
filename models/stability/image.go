package stability

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client
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
// [image.Options].Model selects Core, Ultra, or one exact SD 3.5 model;
// the adapter derives the official endpoint and request model field from it.
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

func (i *ImageModel) buildAPIRequest(req *image.Request) (string, *generateRequest, error) {
	mergedOpts, err := i.defaultOptions.Merged(req.Options)
	if err != nil {
		return "", nil, err
	}
	if err := options.RejectUnsupported("stability: image", map[string]bool{
		"height": mergedOpts.Height != nil,
		"width":  mergedOpts.Width != nil,
	}); err != nil {
		return "", nil, err
	}

	apiReqValue, _, err := metadata.Decode[generateRequest](mergedOpts.Extensions, RequestExtensionKey)

	apiReq := &apiReqValue
	if err != nil {
		return "", nil, err
	}
	endpoint, wireModel, err := resolveModel(mergedOpts.Model)
	if err != nil {
		return "", nil, err
	}
	apiReq.Model = wireModel

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
	if err := validateGenerateRequest(apiReq); err != nil {
		return "", nil, err
	}

	return endpoint, apiReq, nil
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
		if err := resultMeta.Set("stability/finish_reason", envelope.FinishReason); err != nil {
			return nil, err
		}
	}
	if err := resultMeta.Set("stability/seed", envelope.Seed); err != nil {
		return nil, err
	}

	result, err := image.NewResult(value, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &image.ResponseMetadata{}
	if rid := hdr.Get("request-id"); rid != "" {
		if err := meta.Set("stability/request_id", rid); err != nil {
			return nil, err
		}
	}
	if err := meta.Set(ResponseExtensionKey, envelope); err != nil {
		return nil, err
	}

	return image.NewResponse([]*image.Result{result}, meta)
}

func (i *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	endpoint, apiReq, err := i.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	body, hdr, err := i.api.generate(ctx, endpoint, apiReq)
	if err != nil {
		return nil, err
	}

	return i.buildResponse(body, hdr, apiReq.OutputFormat)
}

func resolveModel(model string) (endpoint, wireModel string, err error) {
	switch model {
	case ModelCore:
		return endpointCore, "", nil
	case ModelUltra:
		return endpointUltra, "", nil
	case ModelSD3Point5Large, ModelSD3Point5LargeTurbo, ModelSD3Point5Medium, ModelSD3Point5Flash:
		return endpointSD3, model, nil
	default:
		return "", "", fmt.Errorf("stability: unsupported image model %q", model)
	}
}

func validateGenerateRequest(req *generateRequest) error {
	if req.AspectRatio != "" {
		switch req.AspectRatio {
		case "16:9", "1:1", "21:9", "2:3", "3:2", "4:5", "5:4", "9:16", "9:21":
		default:
			return fmt.Errorf("stability: unsupported aspect_ratio %q", req.AspectRatio)
		}
	}
	if req.OutputFormat != "" && req.OutputFormat != "jpeg" && req.OutputFormat != "png" && req.OutputFormat != "webp" {
		return fmt.Errorf("stability: output_format must be jpeg, png, or webp, got %q", req.OutputFormat)
	}
	if req.Seed != nil && (*req.Seed < 0 || *req.Seed > 4294967294) {
		return fmt.Errorf("stability: seed must be between 0 and 4294967294, got %d", *req.Seed)
	}
	if req.CFGScale != nil && (*req.CFGScale < 1 || *req.CFGScale > 10) {
		return fmt.Errorf("stability: cfg_scale must be between 1 and 10, got %g", *req.CFGScale)
	}
	return nil
}
