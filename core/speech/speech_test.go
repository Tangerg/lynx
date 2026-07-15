package speech_test

import (
	"context"
	"errors"
	"iter"
	"testing"

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
	if _, err := speech.NewOptions(""); err == nil {
		t.Fatal("NewOptions accepted empty model")
	}
	if _, err := speech.NewRequest(""); err == nil {
		t.Fatal("NewRequest accepted empty text")
	}
	if _, err := speech.MergeOptions(nil); err == nil {
		t.Fatal("MergeOptions accepted nil base")
	}
}

func TestResponseValidation(t *testing.T) {
	if _, err := speech.NewResult(nil, &speech.ResultMetadata{}); err == nil {
		t.Fatal("NewResult accepted empty speech")
	}
	if _, err := speech.NewResult([]byte("audio"), nil); err == nil {
		t.Fatal("NewResult accepted nil metadata")
	}
	result, _ := speech.NewResult([]byte("audio"), &speech.ResultMetadata{})
	if _, err := speech.NewResponse(result, &speech.ResponseMetadata{}); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsMergeAndProtocolAccessors(t *testing.T) {
	if _, ok := (*speech.Options)(nil).Get("missing"); ok {
		t.Fatal("nil Options reported a value")
	}
	if clone := (*speech.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}
	base := &speech.Options{Model: "base", Voice: "base-voice", Extra: map[string]any{"base": true}}
	merged, err := speech.MergeOptions(base, nil, &speech.Options{
		Model: "override", Voice: "alloy", ResponseFormat: "mp3", Speed: 1.25,
		Extra: map[string]any{"override": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || merged.Voice != "alloy" || merged.ResponseFormat != "mp3" ||
		merged.Speed != 1.25 || len(merged.Extra) != 2 {
		t.Fatalf("MergeOptions = %#v", merged)
	}
	clone := merged.Clone()
	clone.Extra["base"] = false
	if merged.Extra["base"] != true {
		t.Fatal("Options.Clone aliases source state")
	}
	merged.Set("region", "local")
	if value, ok := merged.Get("region"); !ok || value != "local" {
		t.Fatalf("Options.Get = %#v, %t", value, ok)
	}

	resultMetadata := &speech.ResultMetadata{}
	resultMetadata.Set("duration_ms", 250)
	if value, ok := resultMetadata.Get("duration_ms"); !ok || value != 250 {
		t.Fatalf("ResultMetadata.Get = %#v, %t", value, ok)
	}
	responseMetadata := &speech.ResponseMetadata{}
	responseMetadata.Set("region", "local")
	if value, ok := responseMetadata.Get("region"); !ok || value != "local" {
		t.Fatalf("ResponseMetadata.Get = %#v, %t", value, ok)
	}
	if _, ok := (*speech.ResultMetadata)(nil).Get("missing"); ok {
		t.Fatal("nil ResultMetadata reported a value")
	}
	if _, ok := (*speech.ResponseMetadata)(nil).Get("missing"); ok {
		t.Fatal("nil ResponseMetadata reported a value")
	}
}

func TestResponseAndRequestErrorBoundaries(t *testing.T) {
	if _, err := speech.NewRequest(""); err == nil {
		t.Fatal("NewRequest accepted empty text")
	}
	result, _ := speech.NewResult([]byte("audio"), &speech.ResultMetadata{})
	if _, err := speech.NewResponse(nil, &speech.ResponseMetadata{}); err == nil {
		t.Fatal("NewResponse accepted nil result")
	}
	if _, err := speech.NewResponse(result, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
}
