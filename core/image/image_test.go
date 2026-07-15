package image_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/metadata"
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
	if err := (*image.Request)(nil).Validate(); err == nil {
		t.Fatal("Validate accepted nil request")
	}
	invalid := &image.Request{
		Prompt:  "lynx",
		Options: &image.Options{Extra: metadata.Map{"broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	width := int64(0)
	invalid.Options = &image.Options{Width: &width}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted non-positive width")
	}
	options := new(image.Options)
	if err := options.Set("provider/value", func() {}); err == nil || options.Extra != nil {
		t.Fatalf("failed Set mutated options: %#v, %v", options.Extra, err)
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

func TestOptionsMergeAndCopies(t *testing.T) {
	if clone := (*image.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}

	width, height, seed := int64(512), int64(768), int64(7)
	base := &image.Options{Model: "base", Width: &width, Extra: mustMetadata(t, map[string]any{"base": true})}
	override := &image.Options{
		Model: "override", NegativePrompt: "text", Width: &width, Height: &height,
		Style: "natural", Quality: "high", Seed: &seed, OutputFormat: "IMAGE/PNG",
		ResponseFormat: image.ResponseFormatURL, Extra: mustMetadata(t, map[string]any{"override": true}),
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
	*merged.Height = 1024
	*merged.Seed = 9
	if height != 768 || seed != 7 {
		t.Fatal("MergeOptions aliases override pointer state")
	}
	clone := merged.Clone()
	*clone.Width = 1024
	if err := metadata.Set(clone.Extra, "base", false); err != nil {
		t.Fatal(err)
	}
	if *merged.Width != 512 || !mustDecode[bool](t, merged.Extra, "base") {
		t.Fatal("Options.Clone aliases source state")
	}
}

func TestResponseMetadataAndErrors(t *testing.T) {
	resultMetadata := &image.ResultMetadata{Extra: mustMetadata(t, map[string]any{"revised_prompt": "lynx"})}
	responseMetadata := &image.ResponseMetadata{Extra: mustMetadata(t, map[string]any{"region": "local"})}

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

func mustMetadata(t *testing.T, values map[string]any) metadata.Map {
	t.Helper()
	result, err := metadata.FromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustDecode[T any](t *testing.T, values metadata.Map, key string) T {
	t.Helper()
	value, ok, err := metadata.Decode[T](values, key)
	if err != nil || !ok {
		t.Fatalf("metadata.Decode(%q) = %#v, %t, %v", key, value, ok, err)
	}
	return value
}
