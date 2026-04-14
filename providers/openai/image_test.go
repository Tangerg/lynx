package openai

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/image"
	"github.com/Tangerg/lynx/pkg/assert"
	"github.com/Tangerg/lynx/pkg/mime"
)

const (
	testPromptSimple   = "a beautiful sunset over the ocean"
	testPromptDetailed = "an island near sea, with seagulls, moon shining over the sea, light house, boats in the background, fish flying over the sea"
	testPromptComplex  = "a futuristic city at night with neon lights, flying cars, and tall skyscrapers reflecting in the water below"
	testPromptMinimal  = "a red apple"
)

func newTestImageModel(t *testing.T) *ImageModel {
	t.Helper()

	if currentConfig.imageModel == "" {
		t.Skip("image model not configured")
	}

	defaultOptions := assert.Must(image.NewOptions(currentConfig.imageModel))

	model, err := NewImageModel(
		&ImageModelConfig{
			ApiKey:         getAPIKey(t),
			DefaultOptions: defaultOptions,
			RequestOptions: []option.RequestOption{
				option.WithBaseURL(currentConfig.baseURL),
			},
		},
	)
	if err != nil {
		t.Fatalf("failed to create image model: %v", err)
	}

	return model
}

func TestNewImageModel(t *testing.T) {
	if currentConfig.imageModel == "" {
		t.Skip("image model not configured")
	}

	tests := []struct {
		name           string
		apiKey         model.ApiKey
		defaultOptions *image.Options
		wantErr        bool
		errMsg         string
	}{
		{
			name:           "valid configuration",
			apiKey:         getAPIKey(t),
			defaultOptions: assert.Must(image.NewOptions(currentConfig.imageModel)),
			wantErr:        false,
		},
		{
			name:           "nil api key",
			apiKey:         nil,
			defaultOptions: assert.Must(image.NewOptions(currentConfig.imageModel)),
			wantErr:        true,
			errMsg:         "apiKey is required",
		},
		{
			name:    "nil default options",
			apiKey:  getAPIKey(t),
			wantErr: true,
			errMsg:  "default options cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := NewImageModel(
				&ImageModelConfig{
					ApiKey:         tt.apiKey,
					DefaultOptions: tt.defaultOptions,
					RequestOptions: []option.RequestOption{
						option.WithBaseURL(currentConfig.baseURL),
					},
				},
			)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error message %q, got %q", tt.errMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if model == nil {
				t.Error("expected non-nil model")
			}

			if model.defaultOptions == nil {
				t.Error("expected non-nil default options")
			}

			if model.api == nil {
				t.Error("expected non-nil api")
			}
		})
	}
}

func TestImageModel_Call(t *testing.T) {
	model := newTestImageModel(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		prompt  string
		options *image.Options
		wantErr bool
	}{
		{
			name:    "simple prompt",
			prompt:  testPromptSimple,
			wantErr: false,
		},
		{
			name:    "detailed prompt",
			prompt:  testPromptDetailed,
			wantErr: false,
		},
		{
			name:    "complex prompt",
			prompt:  testPromptComplex,
			wantErr: false,
		},
		{
			name:    "minimal prompt",
			prompt:  testPromptMinimal,
			wantErr: false,
		},
		{
			name:    "empty prompt",
			prompt:  "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := image.NewRequest(tt.prompt)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			if tt.options != nil {
				req.Options = tt.options
			}

			resp, err := model.Call(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}

			if resp.Metadata == nil {
				t.Error("expected non-nil metadata")
			}

			results := resp.Results
			if len(results) == 0 {
				t.Fatal("expected at least one result")
			}

			for i, result := range results {
				if result == nil {
					t.Errorf("result %d is nil", i)
					continue
				}

				if result.Image == nil {
					t.Errorf("result %d has nil image", i)
					continue
				}

				hasURL := result.Image.URL != ""
				hasB64 := result.Image.B64JSON != ""

				if !hasURL && !hasB64 {
					t.Errorf("result %d has neither URL nor B64JSON", i)
				}

				if hasURL {
					t.Logf("Result %d URL: %s", i, result.Image.URL)
					if !strings.HasPrefix(result.Image.URL, "http") {
						t.Errorf("result %d URL does not start with http", i)
					}
				}

				if hasB64 {
					t.Logf("Result %d B64JSON length: %d", i, len(result.Image.B64JSON))
					if len(result.Image.B64JSON) == 0 {
						t.Errorf("result %d B64JSON is empty string", i)
					}
				}

				if result.Metadata == nil {
					t.Errorf("result %d has nil metadata", i)
				}
			}
		})
	}
}

func TestImageModel_Call_WithOptions(t *testing.T) {
	model := newTestImageModel(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		prompt  string
		options *image.Options
		wantErr bool
	}{
		{
			name:   "with custom size",
			prompt: testPromptSimple,
			options: func() *image.Options {
				opts := assert.Must(image.NewOptions(currentConfig.imageModel))
				width := int64(512)
				height := int64(512)
				opts.Width = &width
				opts.Height = &height
				return opts
			}(),
			wantErr: false,
		},
		{
			name:   "with response format url",
			prompt: testPromptSimple,
			options: func() *image.Options {
				opts := assert.Must(image.NewOptions(currentConfig.imageModel))
				opts.ResponseFormat = "url"
				return opts
			}(),
			wantErr: false,
		},
		{
			name:   "with response format b64_json",
			prompt: testPromptSimple,
			options: func() *image.Options {
				opts := assert.Must(image.NewOptions(currentConfig.imageModel))
				opts.ResponseFormat = "b64_json"
				return opts
			}(),
			wantErr: false,
		},
		{
			name:   "with output format png",
			prompt: testPromptSimple,
			options: func() *image.Options {
				opts := assert.Must(image.NewOptions(currentConfig.imageModel))
				opts.OutputFormat = mime.MustNew("image", "png")
				return opts
			}(),
			wantErr: false,
		},
		{
			name:   "with style natural",
			prompt: testPromptSimple,
			options: func() *image.Options {
				opts := assert.Must(image.NewOptions(currentConfig.imageModel))
				opts.Style = "natural"
				return opts
			}(),
			wantErr: false,
		},
		{
			name:   "with style vivid",
			prompt: testPromptSimple,
			options: func() *image.Options {
				opts := assert.Must(image.NewOptions(currentConfig.imageModel))
				opts.Style = "vivid"
				return opts
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := image.NewRequest(tt.prompt)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			req.Options = tt.options

			resp, err := model.Call(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}

			result := resp.Result()
			if result == nil {
				t.Fatal("expected non-nil result")
			}

			t.Logf("Generated image with options: URL=%s, B64JSON length=%d",
				result.Image.URL, len(result.Image.B64JSON))
		})
	}
}

func TestImageModel_Call_MultipleImages(t *testing.T) {
	model := newTestImageModel(t)
	ctx := context.Background()

	req, err := image.NewRequest(testPromptSimple)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := model.Call(ctx, req)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	results := resp.Results
	t.Logf("Generated %d images", len(results))

	for i, result := range results {
		if result.Image == nil {
			t.Errorf("result %d has nil image", i)
			continue
		}
		t.Logf("Image %d: has URL=%v, has B64=%v",
			i, result.Image.URL != "", result.Image.B64JSON != "")
	}
}

func TestImageModel_DefaultOptions(t *testing.T) {
	model := newTestImageModel(t)

	opts := model.DefaultOptions()
	if opts == nil {
		t.Fatal("expected non-nil default options")
	}

	if opts.Model == "" {
		t.Error("expected non-empty model name")
	}

	t.Logf("Default model: %s", opts.Model)

	if opts.Width != nil {
		t.Logf("Default width: %d", *opts.Width)
	}

	if opts.Height != nil {
		t.Logf("Default height: %d", *opts.Height)
	}

	if opts.Style != "" {
		t.Logf("Default style: %s", opts.Style)
	}
}

func TestImageModel_Info(t *testing.T) {
	model := newTestImageModel(t)

	info := model.Info()

	if info.Provider == "" {
		t.Error("expected non-empty provider")
	}

	if info.Provider != Provider {
		t.Errorf("expected provider %q, got %q", Provider, info.Provider)
	}

	t.Logf("Model info: provider=%s", info.Provider)
}

func TestImageModel_PromptVariations(t *testing.T) {
	model := newTestImageModel(t)
	ctx := context.Background()

	prompts := []string{
		"a cat",
		"a cute cat sitting on a windowsill",
		"a majestic orange tabby cat with green eyes, sitting on a wooden windowsill overlooking a garden, golden hour lighting, photorealistic",
	}

	for i, prompt := range prompts {
		t.Run(fmt.Sprintf("prompt_length_%d", len(prompt)), func(t *testing.T) {
			req, err := image.NewRequest(prompt)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := model.Call(ctx, req)
			if err != nil {
				t.Fatalf("call failed: %v", err)
			}

			result := resp.Result()
			if result == nil {
				t.Fatal("expected non-nil result")
			}

			t.Logf("Prompt %d (length %d): generated successfully", i, len(prompt))
		})
	}
}

func TestImageModel_ResponseFormats(t *testing.T) {
	model := newTestImageModel(t)
	ctx := context.Background()

	formats := []string{"url", "b64_json"}

	for _, format := range formats {
		t.Run(fmt.Sprintf("format_%s", format), func(t *testing.T) {
			req, err := image.NewRequest(testPromptSimple)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			opts := assert.Must(image.NewOptions(currentConfig.imageModel))
			opts.ResponseFormat = image.ResponseFormat(format)
			req.Options = opts

			resp, err := model.Call(ctx, req)
			if err != nil {
				t.Fatalf("call failed: %v", err)
			}

			result := resp.Result()
			if result == nil {
				t.Fatal("expected non-nil result")
			}

			switch format {
			case "url":
				if result.Image.URL == "" {
					t.Error("expected non-empty URL for url format")
				}
				t.Logf("URL format: %s", result.Image.URL)
			case "b64_json":
				if result.Image.B64JSON == "" {
					t.Error("expected non-empty B64JSON for b64_json format")
				}
				t.Logf("B64JSON format: length=%d", len(result.Image.B64JSON))
			}
		})
	}
}
