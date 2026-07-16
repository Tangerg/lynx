package embedding_test

import (
	"math"
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
)

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
	merged, err := base.Merged(
		embedding.Options{Model: "override", Dimensions: &dimensions},
	)
	if err != nil || merged.Model != "override" || *merged.Dimensions != 32 {
		t.Fatalf("Merged() = %#v, %v", merged, err)
	}
	*merged.Dimensions = 64
	if dimensions != 32 {
		t.Fatal("Merged aliases override pointer state")
	}
	invalidDimensions := int64(0)
	if _, err := (embedding.Options{Model: "base", Dimensions: &invalidDimensions}).Merged(); err == nil {
		t.Fatal("Merged accepted invalid base options")
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

func TestProtocolConstructorsRejectInvalidValues(t *testing.T) {
	if merged, err := (embedding.Options{}).Merged(); err != nil || merged.Model != "" || merged.Dimensions != nil || len(merged.Extensions) != 0 {
		t.Fatalf("zero Options.Merged() = %#v, %v", merged, err)
	}
	if _, err := embedding.NewResult(nil, &embedding.ResultMetadata{}); err == nil {
		t.Fatal("NewResult accepted an empty vector")
	}
	if _, err := embedding.NewResult([]float64{1}, nil); err == nil {
		t.Fatal("NewResult accepted nil metadata")
	}
	vector := []float64{1}
	result, _ := embedding.NewResult(vector, &embedding.ResultMetadata{})
	vector[0] = 2
	if result.Embedding[0] != 1 {
		t.Fatal("NewResult aliases the input vector")
	}
	if _, err := embedding.NewResponse(nil, &embedding.ResponseMetadata{}); err == nil {
		t.Fatal("NewResponse accepted no results")
	}
	if _, err := embedding.NewResponse([]*embedding.Result{result}, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
	response, _ := embedding.NewResponse([]*embedding.Result{result}, &embedding.ResponseMetadata{})
	if response.First() != result {
		t.Fatal("First did not return the first result")
	}
	if (&embedding.Response{}).First() != nil || (*embedding.Response)(nil).First() != nil {
		t.Fatal("empty response returned a result")
	}
	invalid := &embedding.Response{
		Results:  []*embedding.Result{{Embedding: []float64{math.NaN()}, Metadata: &embedding.ResultMetadata{}}},
		Metadata: &embedding.ResponseMetadata{},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted a non-finite vector")
	}
}
