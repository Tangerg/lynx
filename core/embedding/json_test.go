package embedding_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/scope/core/embedding"
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
	var extensionOptions embedding.Options
	if err := extensionOptions.SetExtension("invalid", true); !errors.Is(err, embedding.ErrInvalidOptions) {
		t.Fatalf("SetExtension error = %v", err)
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

	output, err := embedding.NewOutput([]float64{1}, &embedding.OutputMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	response := embedding.Response{
		Outputs:  []*embedding.Output{output},
		Metadata: &embedding.ResponseMetadata{},
	}
	if err := json.Unmarshal([]byte(`{"outputs":[],"metadata":{}}`), &response); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("Unmarshal Response error = %v", err)
	}
	if response.First() != output {
		t.Fatalf("failed Response decode mutated receiver: %#v", response)
	}
}

func TestResponseJSONRoundTripPreservesValidatedMetadata(t *testing.T) {
	outputMetadata := &embedding.OutputMetadata{}
	if err := outputMetadata.Set("provider/output", map[string]any{"index": 0}); err != nil {
		t.Fatal(err)
	}
	output, err := embedding.NewOutput([]float64{0.25, -0.5}, outputMetadata)
	if err != nil {
		t.Fatal(err)
	}

	responseMetadata := &embedding.ResponseMetadata{
		Model:   "embedding-model",
		Usage:   &embedding.Usage{InputTokens: 7},
		Created: 42,
	}
	if setErr := responseMetadata.Set("provider/response", "request-id"); setErr != nil {
		t.Fatal(setErr)
	}
	response, err := embedding.NewResponse([]*embedding.Output{output}, responseMetadata)
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded embedding.Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, *response) {
		t.Fatalf("round trip = %#v, want %#v", decoded, *response)
	}
}

func TestResponseJSONRejectsInvalidNestedValues(t *testing.T) {
	if err := (*embedding.OutputMetadata)(nil).Set("provider/value", true); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("nil OutputMetadata.Set error = %v", err)
	}
	if err := (*embedding.ResponseMetadata)(nil).Set("provider/value", true); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("nil ResponseMetadata.Set error = %v", err)
	}
	if err := (&embedding.OutputMetadata{}).Set("provider/value", func() {}); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("invalid OutputMetadata.Set error = %v", err)
	}
	if _, err := json.Marshal(embedding.Usage{InputTokens: -1}); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("negative Usage marshal error = %v", err)
	}
	if _, err := json.Marshal(embedding.ResponseMetadata{Model: " model "}); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("invalid ResponseMetadata marshal error = %v", err)
	}
	if _, err := json.Marshal(embedding.ResponseMetadata{Created: -1}); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("negative Created marshal error = %v", err)
	}

	left, err := embedding.NewOutput([]float64{1}, &embedding.OutputMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	right, err := embedding.NewOutput([]float64{1, 2}, &embedding.OutputMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := embedding.NewResponse([]*embedding.Output{left, right}, &embedding.ResponseMetadata{}); !errors.Is(err, embedding.ErrInvalidResponse) {
		t.Fatalf("mixed dimensions error = %v", err)
	}
}
