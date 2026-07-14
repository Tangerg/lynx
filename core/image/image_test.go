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
