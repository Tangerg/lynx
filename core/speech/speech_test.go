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
