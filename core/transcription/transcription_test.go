package transcription_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/transcription"
)

func TestModelFunc(t *testing.T) {
	want := errors.New("boom")
	model := transcription.ModelFunc(func(_ context.Context, request *transcription.Request) (*transcription.Response, error) {
		if request.Audio == nil {
			t.Fatal("audio is nil")
		}
		return nil, want
	})
	audio, err := media.NewBytes("audio/mpeg", []byte("audio"))
	if err != nil {
		t.Fatal(err)
	}
	request, _ := transcription.NewRequest(audio)
	if _, err := model.Call(t.Context(), request); !errors.Is(err, want) {
		t.Fatalf("Call error = %v, want %v", err, want)
	}
}

func TestOptionsAndRequestValidation(t *testing.T) {
	if _, err := transcription.NewOptions(""); err == nil {
		t.Fatal("NewOptions accepted empty model")
	}
	if _, err := transcription.NewOptions(" model "); err == nil {
		t.Fatal("NewOptions accepted model with surrounding whitespace")
	}
	if _, err := transcription.NewRequest(nil); err == nil {
		t.Fatal("NewRequest accepted nil audio")
	}
	if merged, err := (transcription.Options{}).Merged(); err != nil || merged.Model != "" || merged.Language != "" || len(merged.Extensions) != 0 {
		t.Fatalf("zero Options.Merged() = %#v, %v", merged, err)
	}
	if err := (*transcription.Request)(nil).Validate(); err == nil {
		t.Fatal("Validate accepted nil request")
	}
	audio, err := media.NewBytes("audio/mpeg", []byte("audio"))
	if err != nil {
		t.Fatal(err)
	}
	invalid := &transcription.Request{
		Audio:   audio,
		Options: transcription.Options{Extensions: metadata.Map{"provider/broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = transcription.Options{Model: " model "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted model with surrounding whitespace")
	}
	invalid.Options = transcription.Options{Language: " en "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted language with surrounding whitespace")
	}
	options := new(transcription.Options)
	if err := options.SetExtension("provider/value", func() {}); err == nil || options.Extensions != nil {
		t.Fatalf("failed SetExtension mutated options: %#v, %v", options.Extensions, err)
	}
	if _, err := (transcription.Options{Model: " base"}).Merged(); err == nil {
		t.Fatal("Merged accepted invalid base options")
	}
}

func TestResponseValidation(t *testing.T) {
	result, err := transcription.NewResult("", &transcription.ResultMetadata{})
	if err != nil {
		t.Fatalf("NewResult rejected empty transcript: %v", err)
	}
	if _, err := transcription.NewResult("text", nil); err == nil {
		t.Fatal("NewResult accepted nil metadata")
	}
	if _, err := transcription.NewResponse(result, &transcription.ResponseMetadata{}); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsMergeAndCopies(t *testing.T) {
	base := transcription.Options{
		Model:      "base",
		Extensions: mustMetadata(t, map[string]any{"provider/base": true}),
	}
	merged, err := base.Merged(transcription.Options{
		Model: "override", Language: "en",
		Extensions: mustMetadata(t, map[string]any{"provider/override": true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || merged.Language != "en" || len(merged.Extensions) != 2 {
		t.Fatalf("Merged = %#v", merged)
	}
	clone := merged.Clone()
	if err := clone.Extensions.Set("provider/base", false); err != nil {
		t.Fatal(err)
	}
	if !mustDecode[bool](t, merged.Extensions, "provider/base") {
		t.Fatal("Options.Clone aliases source state")
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

func TestResponseErrorBoundaries(t *testing.T) {
	result, _ := transcription.NewResult("lynx", &transcription.ResultMetadata{})
	if _, err := transcription.NewResponse(nil, &transcription.ResponseMetadata{}); err == nil {
		t.Fatal("NewResponse accepted nil result")
	}
	if _, err := transcription.NewResponse(result, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
	if err := (&transcription.Response{Result: result, Metadata: &transcription.ResponseMetadata{Created: -1}}).Validate(); err == nil {
		t.Fatal("Validate accepted a negative creation time")
	}
}
