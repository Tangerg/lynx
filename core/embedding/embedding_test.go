package embedding_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
)

type pointerModel struct{}

func (*pointerModel) Call(context.Context, *embedding.Request) (*embedding.Response, error) {
	return nil, nil
}

func responseFor(texts []string) *embedding.Response {
	results := make([]*embedding.Result, len(texts))
	for i := range texts {
		results[i], _ = embedding.NewResult([]float64{1, 2, 3, 4}, &embedding.ResultMetadata{Index: int64(i)})
	}
	response, _ := embedding.NewResponse(results, &embedding.ResponseMetadata{Model: "fake"})
	return response
}

func TestModelAndClient(t *testing.T) {
	var captured *embedding.Request
	model := embedding.ModelFunc(func(_ context.Context, request *embedding.Request) (*embedding.Response, error) {
		captured = request
		return responseFor(request.Texts), nil
	})
	client, err := embedding.NewClient(model)
	if err != nil {
		t.Fatal(err)
	}
	vectors, response, err := client.EmbedTexts(t.Context(), []string{"a", "b"})
	if err != nil || response == nil || len(vectors) != 2 || len(captured.Texts) != 2 {
		t.Fatalf("EmbedTexts() = %#v, %#v, %v", vectors, response, err)
	}
	vectors[0][0] = 99
	if response.Results[0].Embedding[0] == 99 {
		t.Fatal("returned vectors alias the provider response")
	}

	vector, _, err := client.EmbedText(t.Context(), "one")
	if err != nil || len(vector) != 4 {
		t.Fatalf("EmbedText() = %#v, %v", vector, err)
	}
	if _, _, err := client.EmbedDocuments(t.Context(), []*document.Document{{Text: "doc"}}); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsInvalidBoundaries(t *testing.T) {
	if _, err := embedding.NewClient(nil); err == nil {
		t.Fatal("NewClient accepted nil")
	}
	var typedNil *pointerModel
	if _, err := embedding.NewClient(typedNil); err == nil {
		t.Fatal("NewClient accepted typed nil")
	}
	client, _ := embedding.NewClient(embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
		return nil, nil
	}))
	if _, err := client.Call(t.Context(), nil); err == nil {
		t.Fatal("Call accepted nil request")
	}
	if _, _, err := client.EmbedDocuments(t.Context(), []*document.Document{nil}); err == nil {
		t.Fatal("EmbedDocuments accepted nil document")
	}
	if _, _, err := client.EmbedText(t.Context(), "x"); err == nil {
		t.Fatal("EmbedText accepted nil response")
	}

	want := errors.New("boom")
	failed, _ := embedding.NewClient(embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
		return nil, want
	}))
	if _, _, err := failed.EmbedText(t.Context(), "x"); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
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
	if _, err := embedding.NewRequest(nil); err == nil {
		t.Fatal("NewRequest accepted empty input")
	}
	dimensions := int64(32)
	merged, err := embedding.MergeOptions(
		&embedding.Options{Model: "base"},
		&embedding.Options{Model: "override", Dimensions: &dimensions, EncodingFormat: embedding.EncodingFormatFloat},
	)
	if err != nil || merged.Model != "override" || *merged.Dimensions != 32 {
		t.Fatalf("MergeOptions() = %#v, %v", merged, err)
	}
	if !embedding.EncodingFormatFloat.Valid() || embedding.EncodingFormat("bad").Valid() {
		t.Fatal("EncodingFormat.Valid is inconsistent")
	}
}
