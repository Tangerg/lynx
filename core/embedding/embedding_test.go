package embedding_test

import (
	"context"
	"math"
	"testing"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/metadata"
)

func TestModelFuncAdaptsCall(t *testing.T) {
	request, err := embedding.NewRequest([]string{"scope"})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	model := embedding.ModelFunc(func(_ context.Context, actual *embedding.Request) (*embedding.Response, error) {
		called = true
		if actual != request {
			t.Fatal("ModelFunc received a different request")
		}
		return nil, nil
	})
	if _, err := model.Call(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("ModelFunc did not invoke the adapted function")
	}
}

func TestOptionsAndRequest(t *testing.T) {
	if _, err := embedding.NewOptions(""); err == nil {
		t.Fatal("NewOptions accepted empty model")
	}
	if _, err := embedding.NewOptions(" model "); err == nil {
		t.Fatal("NewOptions accepted model with surrounding whitespace")
	}
	if _, err := embedding.NewRequest(nil); err == nil {
		t.Fatal("NewRequest accepted empty input")
	}
	if _, err := embedding.NewRequest([]string{"valid", ""}); err == nil {
		t.Fatal("NewRequest accepted an empty text entry")
	}
	if err := (*embedding.Request)(nil).Validate(); err == nil {
		t.Fatal("Validate accepted nil request")
	}
	invalid := &embedding.Request{
		Texts:   []string{"text"},
		Options: embedding.Options{Extensions: metadata.Map{"provider/broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = embedding.Options{Model: " model "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted model with surrounding whitespace")
	}
	badDimensions := int64(0)
	invalid.Options = embedding.Options{Dimensions: &badDimensions}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted non-positive dimensions")
	}
	options := new(embedding.Options)
	if err := options.SetExtension("provider/value", func() {}); err == nil || options.Extensions != nil {
		t.Fatalf("failed SetExtension mutated options: %#v, %v", options.Extensions, err)
	}
	dimensions := int64(32)
	base := embedding.Options{Model: "base"}
	resolved, err := base.Resolve(
		embedding.Options{Model: "override", Dimensions: &dimensions},
	)
	if err != nil || resolved.Model != "override" || *resolved.Dimensions != 32 {
		t.Fatalf("Resolve(override) = %#v, %v", resolved, err)
	}
	*resolved.Dimensions = 64
	if dimensions != 32 {
		t.Fatal("Resolve aliases override pointer state")
	}
	invalidDimensions := int64(0)
	if _, err := (embedding.Options{Model: "base", Dimensions: &invalidDimensions}).Resolve(embedding.Options{}); err == nil {
		t.Fatal("Resolve accepted invalid base options")
	}
}

func TestProtocolValueCopies(t *testing.T) {
	dimensions := int64(64)
	options := embedding.Options{
		Model: "base", Dimensions: &dimensions,
		Extensions: mustMetadata(t, map[string]any{"provider/region": "local"}),
	}
	clone := options.Clone()
	*clone.Dimensions = 128
	if err := clone.Extensions.Set("provider/region", "remote"); err != nil {
		t.Fatal(err)
	}
	if *options.Dimensions != 64 || mustDecode[string](t, options.Extensions, "provider/region") != "local" {
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

func TestProtocolConstructorsRejectInvalidValues(t *testing.T) {
	if resolved, err := (embedding.Options{}).Resolve(embedding.Options{}); err != nil || resolved.Model != "" || resolved.Dimensions != nil || len(resolved.Extensions) != 0 {
		t.Fatalf("zero Options.Resolve(empty) = %#v, %v", resolved, err)
	}
	if _, err := embedding.NewOutput(nil, &embedding.OutputMetadata{}); err == nil {
		t.Fatal("NewOutput accepted an empty vector")
	}
	if _, err := embedding.NewOutput([]float64{1}, nil); err == nil {
		t.Fatal("NewOutput accepted nil metadata")
	}
	vector := []float64{1}
	output, _ := embedding.NewOutput(vector, &embedding.OutputMetadata{})
	vector[0] = 2
	if output.Embedding[0] != 1 {
		t.Fatal("NewOutput aliases the input vector")
	}
	if _, err := embedding.NewResponse(nil, &embedding.ResponseMetadata{}); err == nil {
		t.Fatal("NewResponse accepted no outputs")
	}
	if _, err := embedding.NewResponse([]*embedding.Output{output}, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
	response, _ := embedding.NewResponse([]*embedding.Output{output}, &embedding.ResponseMetadata{})
	if response.First() != output {
		t.Fatal("First did not return the first output")
	}
	if (&embedding.Response{}).First() != nil || (*embedding.Response)(nil).First() != nil {
		t.Fatal("empty response returned a output")
	}
	invalid := &embedding.Response{
		Outputs:  []*embedding.Output{{Embedding: []float64{math.NaN()}, Metadata: &embedding.OutputMetadata{}}},
		Metadata: &embedding.ResponseMetadata{},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted a non-finite vector")
	}
}
