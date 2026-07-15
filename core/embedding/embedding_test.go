package embedding_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
)

func responseFor(texts []string) *embedding.Response {
	results := make([]*embedding.Result, len(texts))
	for i := range texts {
		results[i], _ = embedding.NewResult([]float64{1, 2, 3, 4}, &embedding.ResultMetadata{Index: int64(i)})
	}
	response, _ := embedding.NewResponse(results, &embedding.ResponseMetadata{Model: "fake"})
	return response
}

func TestResolveDimensions(t *testing.T) {
	explicit := struct {
		embedding.Model
		embedding.Dimensioner
	}{
		Model: embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
			t.Fatal("explicit Dimensioner must not probe")
			return nil, nil
		}),
		Dimensioner: embedding.DimensionFunc(func(context.Context) (int, error) { return 8, nil }),
	}
	if got, err := embedding.ResolveDimensions(t.Context(), explicit); err != nil || got != 8 {
		t.Fatalf("explicit dimensions = %d, %v", got, err)
	}

	probe := embedding.ModelFunc(func(_ context.Context, request *embedding.Request) (*embedding.Response, error) {
		return responseFor(request.Texts), nil
	})
	if got, err := embedding.ResolveDimensions(t.Context(), probe); err != nil || got != 4 {
		t.Fatalf("probed dimensions = %d, %v", got, err)
	}

	bad := struct {
		embedding.Model
		embedding.Dimensioner
	}{probe, embedding.DimensionFunc(func(context.Context) (int, error) { return 0, nil })}
	if _, err := embedding.ResolveDimensions(t.Context(), bad); err == nil {
		t.Fatal("ResolveDimensions accepted zero")
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
		Options: &embedding.Options{Extra: metadata.Map{"broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = &embedding.Options{Model: " model "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted model with surrounding whitespace")
	}
	badDimensions := int64(0)
	invalid.Options = &embedding.Options{Dimensions: &badDimensions}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted non-positive dimensions")
	}
	options := new(embedding.Options)
	if err := options.Set("provider/value", func() {}); err == nil || options.Extra != nil {
		t.Fatalf("failed Set mutated options: %#v, %v", options.Extra, err)
	}
	dimensions := int64(32)
	base := &embedding.Options{Model: "base"}
	merged, err := base.Merged(
		&embedding.Options{Model: "override", Dimensions: &dimensions, EncodingFormat: embedding.EncodingFormatFloat},
	)
	if err != nil || merged.Model != "override" || *merged.Dimensions != 32 {
		t.Fatalf("Merged() = %#v, %v", merged, err)
	}
	*merged.Dimensions = 64
	if dimensions != 32 {
		t.Fatal("Merged aliases override pointer state")
	}
	if !embedding.EncodingFormatFloat.Valid() || embedding.EncodingFormat("bad").Valid() {
		t.Fatal("EncodingFormat.Valid is inconsistent")
	}
}

func TestProtocolValueCopies(t *testing.T) {
	if clone := (*embedding.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}

	dimensions := int64(64)
	options := &embedding.Options{
		Model: "base", Dimensions: &dimensions,
		Extra: mustMetadata(t, map[string]any{"region": "local"}),
	}
	clone := options.Clone()
	*clone.Dimensions = 128
	if err := clone.Extra.Set("region", "remote"); err != nil {
		t.Fatal(err)
	}
	if *options.Dimensions != 64 || mustDecode[string](t, options.Extra, "region") != "local" {
		t.Fatal("Options.Clone aliases source state")
	}
	if embedding.Image.String() != "image" {
		t.Fatalf("ModalityType.String = %q", embedding.Image.String())
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
	if _, err := (*embedding.Options)(nil).Merged(); err == nil {
		t.Fatal("Merged accepted nil receiver")
	}
	if _, err := embedding.NewResult(nil, &embedding.ResultMetadata{}); err == nil {
		t.Fatal("NewResult accepted an empty vector")
	}
	if _, err := embedding.NewResult([]float64{1}, nil); err == nil {
		t.Fatal("NewResult accepted nil metadata")
	}
	result, _ := embedding.NewResult([]float64{1}, &embedding.ResultMetadata{})
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
}
