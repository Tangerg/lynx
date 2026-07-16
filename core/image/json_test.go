package image_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
)

func TestJSONBoundaries(t *testing.T) {
	if _, err := image.NewOptions(""); !errors.Is(err, image.ErrInvalidOptions) {
		t.Fatalf("NewOptions error = %v", err)
	}
	if _, err := image.NewRequest(""); !errors.Is(err, image.ErrInvalidRequest) {
		t.Fatalf("NewRequest error = %v", err)
	}
	if _, err := image.NewResponse(nil, &image.ResponseMetadata{}); !errors.Is(err, image.ErrInvalidResponse) {
		t.Fatalf("NewResponse error = %v", err)
	}
	var extensionOptions image.Options
	if err := extensionOptions.SetExtension("invalid", true); !errors.Is(err, image.ErrInvalidOptions) {
		t.Fatalf("SetExtension error = %v", err)
	}

	if _, err := json.Marshal(image.Options{Model: " invalid "}); !errors.Is(err, image.ErrInvalidOptions) {
		t.Fatalf("Marshal Options error = %v", err)
	}
	if _, err := json.Marshal(image.Request{}); !errors.Is(err, image.ErrInvalidRequest) {
		t.Fatalf("Marshal Request error = %v", err)
	}
	if _, err := json.Marshal(image.Response{}); !errors.Is(err, image.ErrInvalidResponse) {
		t.Fatalf("Marshal Response error = %v", err)
	}

	options := image.Options{Model: "keep"}
	if err := json.Unmarshal([]byte(`{"model":" invalid "}`), &options); !errors.Is(err, image.ErrInvalidOptions) {
		t.Fatalf("Unmarshal Options error = %v", err)
	}
	if options.Model != "keep" {
		t.Fatalf("failed Options decode mutated receiver: %#v", options)
	}

	request := image.Request{Prompt: "keep"}
	if err := json.Unmarshal([]byte(`{"prompt":""}`), &request); !errors.Is(err, image.ErrInvalidRequest) {
		t.Fatalf("Unmarshal Request error = %v", err)
	}
	if request.Prompt != "keep" {
		t.Fatalf("failed Request decode mutated receiver: %#v", request)
	}

	generated, err := media.NewURI("image/png", "https://example.com/image.png")
	if err != nil {
		t.Fatal(err)
	}
	result, err := image.NewResult(generated, &image.ResultMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	response := image.Response{
		Results:  []*image.Result{result},
		Metadata: &image.ResponseMetadata{},
	}
	if err := json.Unmarshal([]byte(`{"results":[],"metadata":{}}`), &response); !errors.Is(err, image.ErrInvalidResponse) {
		t.Fatalf("Unmarshal Response error = %v", err)
	}
	if response.First() != result {
		t.Fatalf("failed Response decode mutated receiver: %#v", response)
	}
}
