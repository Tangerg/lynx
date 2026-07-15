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
	if _, err := (*transcription.Options)(nil).Merged(); err == nil {
		t.Fatal("Merged accepted nil receiver")
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
		Options: &transcription.Options{Extra: metadata.Map{"broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = &transcription.Options{Model: " model "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted model with surrounding whitespace")
	}
	invalid.Options = &transcription.Options{Language: " en "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted language with surrounding whitespace")
	}
	options := new(transcription.Options)
	if err := options.Set("provider/value", func() {}); err == nil || options.Extra != nil {
		t.Fatalf("failed Set mutated options: %#v, %v", options.Extra, err)
	}
	if _, err := (&transcription.Options{Model: " base"}).Merged(); err == nil {
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
	if clone := (*transcription.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}
	base := &transcription.Options{
		Model: "base",
		Extra: mustMetadata(t, map[string]any{"base": true}),
	}
	merged, err := base.Merged(nil, &transcription.Options{
		Model: "override", Language: "en",
		Extra: mustMetadata(t, map[string]any{"override": true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || merged.Language != "en" || len(merged.Extra) != 2 {
		t.Fatalf("Merged = %#v", merged)
	}
	clone := merged.Clone()
	if err := clone.Extra.Set("base", false); err != nil {
		t.Fatal(err)
	}
	if !mustDecode[bool](t, merged.Extra, "base") {
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
