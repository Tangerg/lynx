package embedding_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
)

func TestJSONBoundaries(t *testing.T) {
	if _, err := embedding.NewOptions(""); !errors.Is(err, embedding.ErrInvalidOptions) {
		t.Fatalf("NewOptions error = %v", err)
	}
	if _, err := embedding.NewRequest(nil); !errors.Is(err, embedding.ErrInvalidRequest) {
		t.Fatalf("NewRequest error = %v", err)
	}
	if _, err := embedding.NewResponse(nil, &embedding.ResponseMetadata{}); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("NewResponse error = %v", err)
	}

	if _, err := json.Marshal(embedding.Options{Model: " invalid "}); !errors.Is(err, embedding.ErrInvalidOptions) {
		t.Fatalf("Marshal Options error = %v", err)
	}
	if _, err := json.Marshal(embedding.Request{}); !errors.Is(err, embedding.ErrInvalidRequest) {
		t.Fatalf("Marshal Request error = %v", err)
	}
	if _, err := json.Marshal(embedding.Response{}); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("Marshal Response error = %v", err)
	}

	options := embedding.Options{Model: "keep"}
	if err := json.Unmarshal([]byte(`{"model":" invalid "}`), &options); !errors.Is(err, embedding.ErrInvalidOptions) {
		t.Fatalf("Unmarshal Options error = %v", err)
	}
	if options.Model != "keep" {
		t.Fatalf("failed Options decode mutated receiver: %#v", options)
	}

	request := embedding.Request{Texts: []string{"keep"}}
	if err := json.Unmarshal([]byte(`{"texts":[]}`), &request); !errors.Is(err, embedding.ErrInvalidRequest) {
		t.Fatalf("Unmarshal Request error = %v", err)
	}
	if len(request.Texts) != 1 || request.Texts[0] != "keep" {
		t.Fatalf("failed Request decode mutated receiver: %#v", request)
	}

	result, err := embedding.NewResult([]float64{1}, &embedding.ResultMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	response := embedding.Response{
		Results:  []*embedding.Result{result},
		Metadata: &embedding.ResponseMetadata{},
	}
	if err := json.Unmarshal([]byte(`{"results":[],"metadata":{}}`), &response); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("Unmarshal Response error = %v", err)
	}
	if response.First() != result {
		t.Fatalf("failed Response decode mutated receiver: %#v", response)
	}
}
