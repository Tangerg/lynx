package image_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
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
	if _, err := image.NewOptions(" model "); err == nil {
		t.Fatal("NewOptions accepted model with surrounding whitespace")
	}
	if _, err := image.NewRequest(""); err == nil {
		t.Fatal("NewRequest accepted empty prompt")
	}
	if _, err := (*image.Options)(nil).Merged(); err == nil {
		t.Fatal("Merged accepted nil receiver")
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
	invalid.Options = &image.Options{Model: " model "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted model with surrounding whitespace")
	}
	width := int64(0)
	invalid.Options = &image.Options{Width: &width}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted non-positive width")
	}
	seed := int64(-1)
	invalid.Options = &image.Options{Seed: &seed}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted a negative seed")
	}
	invalid.Options = &image.Options{OutputFormat: "IMAGE/PNG"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted a non-canonical output MIME type")
	}
	options := new(image.Options)
	if err := options.Set("provider/value", func() {}); err == nil || options.Extra != nil {
		t.Fatalf("failed Set mutated options: %#v, %v", options.Extra, err)
	}
	base, err := image.NewOptions("image-model")
	if err != nil {
		t.Fatal(err)
	}
	base.OutputFormat = "IMAGE/PNG"
	merged, err := base.Merged()
	if err != nil {
		t.Fatal(err)
	}
	if merged.OutputFormat != "image/png" {
		t.Fatalf("normalized OutputFormat = %q, want image/png", merged.OutputFormat)
	}
	for _, invalid := range []string{"text/plain", "image", "image/png;charset=utf-8"} {
		base.OutputFormat = invalid
		if _, err := base.Merged(); err == nil {
			t.Errorf("Merged accepted invalid OutputFormat %q", invalid)
		}
	}
	if _, err := (&image.Options{Model: " model "}).Merged(); err == nil {
		t.Fatal("Merged accepted invalid base options")
	}
}

func TestResponseValidation(t *testing.T) {
	generated, err := media.NewURI("image/png", "https://example.com/image.png")
	if err != nil {
		t.Fatal(err)
	}
	result, err := image.NewResult(generated, &image.ResultMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := image.NewResponse([]*image.Result{result}, &image.ResponseMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if response.First() != result {
		t.Fatal("First did not return the first result")
	}
	if (&image.Response{}).First() != nil || (*image.Response)(nil).First() != nil {
		t.Fatal("empty response returned a result")
	}
	response.Metadata.Created = -1
	if err := response.Validate(); err == nil {
		t.Fatal("Validate accepted a negative creation time")
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
		Seed: &seed, OutputFormat: "IMAGE/PNG",
		Extra: mustMetadata(t, map[string]any{"override": true}),
	}
	merged, err := base.Merged(nil, override)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || merged.NegativePrompt != "text" || merged.Height == nil ||
		merged.Seed == nil || merged.OutputFormat != "image/png" {
		t.Fatalf("Merged = %#v", merged)
	}
	if len(merged.Extra) != 2 {
		t.Fatalf("merged Extra = %#v", merged.Extra)
	}
	*merged.Height = 1024
	*merged.Seed = 9
	if height != 768 || seed != 7 {
		t.Fatal("Merged aliases override pointer state")
	}
	clone := merged.Clone()
	*clone.Width = 1024
	if err := clone.Extra.Set("base", false); err != nil {
		t.Fatal(err)
	}
	if *merged.Width != 512 || !mustDecode[bool](t, merged.Extra, "base") {
		t.Fatal("Options.Clone aliases source state")
	}
}

func TestResponseMetadataAndErrors(t *testing.T) {
	resultMetadata := &image.ResultMetadata{Extra: mustMetadata(t, map[string]any{"revised_prompt": "lynx"})}
	responseMetadata := &image.ResponseMetadata{Extra: mustMetadata(t, map[string]any{"region": "local"})}

	generated, _ := media.NewURI("image/png", "https://example.com/image.png")
	if _, err := image.NewResult(nil, resultMetadata); err == nil {
		t.Fatal("NewResult accepted nil media")
	}
	if _, err := image.NewResult(generated, nil); err == nil {
		t.Fatal("NewResult accepted nil metadata")
	}
	audio, _ := media.NewBytes("audio/mpeg", []byte("audio"))
	if _, err := image.NewResult(audio, resultMetadata); err == nil {
		t.Fatal("NewResult accepted non-image media")
	}
	result, _ := image.NewResult(generated, resultMetadata)
	if _, err := image.NewResponse(nil, responseMetadata); err == nil {
		t.Fatal("NewResponse accepted no results")
	}
	if _, err := image.NewResponse([]*image.Result{nil}, responseMetadata); err == nil {
		t.Fatal("NewResponse accepted nil result")
	}
	if _, err := image.NewResponse([]*image.Result{result}, nil); err == nil {
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
