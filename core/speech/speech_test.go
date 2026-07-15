package speech_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/speech"
)

func TestModelAndStreamFunc(t *testing.T) {
	want := errors.New("boom")
	model := speech.ModelFunc(func(_ context.Context, request *speech.Request) (*speech.Response, error) {
		if request.Text != "hello" {
			t.Fatalf("text = %q", request.Text)
		}
		return nil, want
	})
	request, _ := speech.NewRequest("hello")
	if _, err := model.Call(t.Context(), request); !errors.Is(err, want) {
		t.Fatalf("Call error = %v, want %v", err, want)
	}

	streamer := speech.StreamFunc(func(context.Context, *speech.Request) iter.Seq2[*speech.Response, error] {
		return func(yield func(*speech.Response, error) bool) {
			yield(nil, want)
		}
	})
	for _, err := range streamer.Stream(t.Context(), request) {
		if !errors.Is(err, want) {
			t.Fatalf("Stream error = %v, want %v", err, want)
		}
		return
	}
	t.Fatal("Stream yielded nothing")
}

func TestOptionsAndRequestValidation(t *testing.T) {
	if _, err := speech.NewOptions(""); err == nil || err.Error() != "speech.NewOptions: model id must not be empty" {
		t.Fatalf("NewOptions error = %v", err)
	}
	if _, err := speech.NewRequest(""); err == nil {
		t.Fatal("NewRequest accepted empty text")
	}
	if _, err := speech.MergeOptions(nil); err == nil || err.Error() != "speech.MergeOptions: base options must not be nil" {
		t.Fatalf("MergeOptions error = %v", err)
	}
	if err := (*speech.Request)(nil).Validate(); err == nil {
		t.Fatal("Validate accepted nil request")
	}
	invalid := &speech.Request{
		Text:    "text",
		Options: &speech.Options{Extra: metadata.Map{"broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = &speech.Options{Speed: -1}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted negative speed")
	}
	options := new(speech.Options)
	if err := options.Set("provider/value", func() {}); err == nil || options.Extra != nil {
		t.Fatalf("failed Set mutated options: %#v, %v", options.Extra, err)
	}
}

func TestResponseValidation(t *testing.T) {
	if _, err := speech.NewResult(nil, &speech.ResultMetadata{}); err == nil || err.Error() != "speech.NewResult: speech must not be empty" {
		t.Fatalf("NewResult empty speech error = %v", err)
	}
	if _, err := speech.NewResult([]byte("audio"), nil); err == nil || err.Error() != "speech.NewResult: metadata must not be nil" {
		t.Fatalf("NewResult nil metadata error = %v", err)
	}
	result, _ := speech.NewResult([]byte("audio"), &speech.ResultMetadata{})
	if _, err := speech.NewResponse(result, &speech.ResponseMetadata{}); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsMergeAndCopies(t *testing.T) {
	if clone := (*speech.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}
	base := &speech.Options{Model: "base", Voice: "base-voice", Extra: mustMetadata(t, map[string]any{"base": true})}
	merged, err := speech.MergeOptions(base, nil, &speech.Options{
		Model: "override", Voice: "alloy", ResponseFormat: "mp3", Speed: 1.25,
		Extra: mustMetadata(t, map[string]any{"override": true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || merged.Voice != "alloy" || merged.ResponseFormat != "mp3" ||
		merged.Speed != 1.25 || len(merged.Extra) != 2 {
		t.Fatalf("MergeOptions = %#v", merged)
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

func TestResponseAndRequestErrorBoundaries(t *testing.T) {
	if _, err := speech.NewRequest(""); err == nil {
		t.Fatal("NewRequest accepted empty text")
	}
	result, _ := speech.NewResult([]byte("audio"), &speech.ResultMetadata{})
	if _, err := speech.NewResponse(nil, &speech.ResponseMetadata{}); err == nil || err.Error() != "speech.NewResponse: result must not be nil" {
		t.Fatalf("NewResponse nil result error = %v", err)
	}
	if _, err := speech.NewResponse(result, nil); err == nil || err.Error() != "speech.NewResponse: metadata must not be nil" {
		t.Fatalf("NewResponse nil metadata error = %v", err)
	}
}
