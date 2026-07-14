package transcription_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/media"
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
	if _, err := transcription.NewRequest(nil); err == nil {
		t.Fatal("NewRequest accepted nil audio")
	}
	if _, err := transcription.MergeOptions(nil); err == nil {
		t.Fatal("MergeOptions accepted nil base")
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
