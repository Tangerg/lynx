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
	if resolved, err := (transcription.Options{}).Resolve(transcription.Options{}); err != nil || resolved.Model != "" || resolved.Language != "" || len(resolved.Extensions) != 0 {
		t.Fatalf("zero Options.Resolve(empty) = %#v, %v", resolved, err)
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
	if _, err := (transcription.Options{Model: " base"}).Resolve(transcription.Options{}); err == nil {
		t.Fatal("Resolve accepted invalid base options")
	}
}

func TestResponseValidation(t *testing.T) {
	output, err := transcription.NewOutput("", &transcription.OutputMetadata{})
	if err != nil {
		t.Fatalf("NewOutput rejected empty transcript: %v", err)
	}
	if _, err := transcription.NewOutput("text", nil); err == nil {
		t.Fatal("NewOutput accepted nil metadata")
	}
	if _, err := transcription.NewResponse(output, &transcription.ResponseMetadata{}); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsResolveAndCopies(t *testing.T) {
	base := transcription.Options{
		Model:      "base",
		Extensions: mustMetadata(t, map[string]any{"provider/base": true}),
	}
	resolved, err := base.Resolve(transcription.Options{
		Model: "override", Language: "en",
		Extensions: mustMetadata(t, map[string]any{"provider/override": true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model != "override" || resolved.Language != "en" || len(resolved.Extensions) != 2 {
		t.Fatalf("Resolve = %#v", resolved)
	}
	clone := resolved.Clone()
	if err := clone.Extensions.Set("provider/base", false); err != nil {
		t.Fatal(err)
	}
	if !mustDecode[bool](t, resolved.Extensions, "provider/base") {
		t.Fatal("Options.Clone aliases source state")
	}
}

func mustMetadata(t *testing.T, values map[string]any) metadata.Map {
	t.Helper()
	output, err := metadata.FromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func mustDecode[T any](t *testing.T, values metadata.Map, key string) T {
	t.Helper()
	value, ok, err := values.Decode[T](key)
	if err != nil || !ok {
		t.Fatalf("metadata.Decode(%q) = %#v, %t, %v", key, value, ok, err)
	}
	return value
}

func TestResponseErrorBoundaries(t *testing.T) {
	output, _ := transcription.NewOutput("lynx", &transcription.OutputMetadata{})
	if _, err := transcription.NewResponse(nil, &transcription.ResponseMetadata{}); err == nil {
		t.Fatal("NewResponse accepted nil output")
	}
	if _, err := transcription.NewResponse(output, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
	if err := (&transcription.Response{Output: output, Metadata: &transcription.ResponseMetadata{Created: -1}}).Validate(); err == nil {
		t.Fatal("Validate accepted a negative creation time")
	}
}
