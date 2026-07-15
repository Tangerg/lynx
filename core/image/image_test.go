package image_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/image"
)

func TestModelFunc(t *testing.T) {
	want := errors.New("boom")
	model := image.ModelFunc(func(_ context.Context, request *image.Request) (*image.Response, error) {
		if request.Prompt != "a duck" {
			t.Fatalf("prompt = %q", request.Prompt)
		}
		return nil, want
	})
	request, _ := image.NewRequest("a duck")
	if _, err := model.Call(t.Context(), request); !errors.Is(err, want) {
		t.Fatalf("Call error = %v, want %v", err, want)
	}
}

func TestOptionsAndRequestValidation(t *testing.T) {
	if _, err := image.NewOptions(""); err == nil {
		t.Fatal("NewOptions accepted empty model")
	}
	if _, err := image.NewRequest(""); err == nil {
		t.Fatal("NewRequest accepted empty prompt")
	}
	if _, err := image.MergeOptions(nil); err == nil {
		t.Fatal("MergeOptions accepted nil base")
	}
	if !image.ResponseFormatURL.Valid() || image.ResponseFormat("garbage").Valid() {
		t.Fatal("ResponseFormat.Valid is inconsistent")
	}
	base, err := image.NewOptions("image-model")
	if err != nil {
		t.Fatal(err)
	}
	base.OutputFormat = "IMAGE/PNG"
	merged, err := image.MergeOptions(base)
	if err != nil {
		t.Fatal(err)
	}
	if merged.OutputFormat != "image/png" {
		t.Fatalf("normalized OutputFormat = %q, want image/png", merged.OutputFormat)
	}
	for _, invalid := range []string{"text/plain", "image", "image/png;charset=utf-8"} {
		base.OutputFormat = invalid
		if _, err := image.MergeOptions(base); err == nil {
			t.Errorf("MergeOptions accepted invalid OutputFormat %q", invalid)
		}
	}
}

func TestResponseValidation(t *testing.T) {
	if _, err := image.NewImage("", ""); err == nil {
		t.Fatal("NewImage accepted an empty payload")
	}
	generated, err := image.NewImage("https://example.com/image.png", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := image.NewResult(generated, &image.ResultMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := image.NewResponse(result, &image.ResponseMetadata{}); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsMergeAndProtocolAccessors(t *testing.T) {
	if _, ok := (*image.Options)(nil).Get("missing"); ok {
		t.Fatal("nil Options reported a value")
	}
	if _, ok := (*image.Request)(nil).Get("missing"); ok {
		t.Fatal("nil Request reported a value")
	}
	if clone := (*image.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}

	width, height, seed := int64(512), int64(768), int64(7)
	base := &image.Options{Model: "base", Width: &width, Extra: map[string]any{"base": true}}
	override := &image.Options{
		Model: "override", NegativePrompt: "text", Width: &width, Height: &height,
		Style: "natural", Quality: "high", Seed: &seed, OutputFormat: "IMAGE/PNG",
		ResponseFormat: image.ResponseFormatURL, Extra: map[string]any{"override": true},
	}
	merged, err := image.MergeOptions(base, nil, override)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || merged.NegativePrompt != "text" || merged.Height == nil ||
		merged.Style != "natural" || merged.Quality != "high" || merged.Seed == nil ||
		merged.OutputFormat != "image/png" || merged.ResponseFormat != image.ResponseFormatURL {
		t.Fatalf("MergeOptions = %#v", merged)
	}
	if len(merged.Extra) != 2 {
		t.Fatalf("merged Extra = %#v", merged.Extra)
	}
	clone := merged.Clone()
	*clone.Width = 1024
	clone.Extra["base"] = false
	if *merged.Width != 512 || merged.Extra["base"] != true {
		t.Fatal("Options.Clone aliases source state")
	}
	merged.Set("region", "local")
	if value, ok := merged.Get("region"); !ok || value != "local" {
		t.Fatalf("Options.Get = %#v, %t", value, ok)
	}

	request, _ := image.NewRequest("lynx")
	request.Set("trace_id", "trace-1")
	if value, ok := request.Get("trace_id"); !ok || value != "trace-1" {
		t.Fatalf("Request.Get = %#v, %t", value, ok)
	}
}

func TestResponseMetadataAccessorsAndErrors(t *testing.T) {
	if _, ok := (*image.ResultMetadata)(nil).Get("missing"); ok {
		t.Fatal("nil ResultMetadata reported a value")
	}
	if _, ok := (*image.ResponseMetadata)(nil).Get("missing"); ok {
		t.Fatal("nil ResponseMetadata reported a value")
	}
	resultMetadata := &image.ResultMetadata{}
	resultMetadata.Set("revised_prompt", "lynx")
	if value, ok := resultMetadata.Get("revised_prompt"); !ok || value != "lynx" {
		t.Fatalf("ResultMetadata.Get = %#v, %t", value, ok)
	}
	responseMetadata := &image.ResponseMetadata{}
	responseMetadata.Set("region", "local")
	if value, ok := responseMetadata.Get("region"); !ok || value != "local" {
		t.Fatalf("ResponseMetadata.Get = %#v, %t", value, ok)
	}

	generated, _ := image.NewImage("https://example.com/image.png", "")
	if _, err := image.NewResult(nil, resultMetadata); err == nil {
		t.Fatal("NewResult accepted nil image")
	}
	if _, err := image.NewResult(generated, nil); err == nil {
		t.Fatal("NewResult accepted nil metadata")
	}
	result, _ := image.NewResult(generated, resultMetadata)
	if _, err := image.NewResponse(nil, responseMetadata); err == nil {
		t.Fatal("NewResponse accepted nil result")
	}
	if _, err := image.NewResponse(result, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
}
