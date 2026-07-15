package embeddingclient_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/embeddingclient"
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

func TestClientEmbedsIndependentVectors(t *testing.T) {
	var captured *embedding.Request
	var providerResponses []*embedding.Response
	model := embedding.ModelFunc(func(_ context.Context, request *embedding.Request) (*embedding.Response, error) {
		captured = request
		response := responseFor(request.Texts)
		providerResponses = append(providerResponses, response)
		return response, nil
	})
	client, err := embeddingclient.New(model)
	if err != nil {
		t.Fatal(err)
	}

	vectors, err := client.EmbedTexts(t.Context(), []string{"a", "b"})
	if err != nil || len(vectors) != 2 || len(captured.Texts) != 2 {
		t.Fatalf("EmbedTexts() = %#v, %v", vectors, err)
	}
	vectors[0][0] = 99
	if providerResponses[0].Results[0].Embedding[0] == 99 {
		t.Fatal("returned vectors alias the provider response")
	}
	if vector, err := client.EmbedText(t.Context(), "one"); err != nil || len(vector) != 4 {
		t.Fatalf("EmbedText() = %#v, %v", vector, err)
	}
	if _, err := client.EmbedDocuments(t.Context(), []*document.Document{{Text: "doc"}}); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsInvalidBoundaries(t *testing.T) {
	if _, err := embeddingclient.New(nil); !errors.Is(err, embeddingclient.ErrNilModel) {
		t.Fatalf("New(nil) error = %v, want ErrNilModel", err)
	}
	var typedNil *pointerModel
	if _, err := embeddingclient.New(typedNil); !errors.Is(err, embeddingclient.ErrNilModel) {
		t.Fatalf("New(typed nil) error = %v, want ErrNilModel", err)
	}

	var nilClient *embeddingclient.Client
	if _, err := nilClient.EmbedText(t.Context(), "text"); err == nil {
		t.Fatal("nil Client accepted EmbedText")
	}

	nilResponse, _ := embeddingclient.New(embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
		return nil, nil
	}))
	if _, err := nilResponse.EmbedText(t.Context(), "text"); err == nil {
		t.Fatal("EmbedText accepted a nil response")
	}

	want := errors.New("boom")
	failed, _ := embeddingclient.New(embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
		return nil, want
	}))
	if _, err := failed.EmbedText(t.Context(), "text"); !errors.Is(err, want) {
		t.Fatalf("EmbedText error = %v, want %v", err, want)
	}
}

func TestClientValidatesResultsAndDocuments(t *testing.T) {
	tests := []struct {
		name     string
		response *embedding.Response
	}{
		{name: "wrong result count", response: &embedding.Response{}},
		{name: "nil result", response: &embedding.Response{Results: []*embedding.Result{nil}}},
		{name: "empty vector", response: &embedding.Response{Results: []*embedding.Result{{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := embeddingclient.New(embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
				return tt.response, nil
			}))
			if _, err := client.EmbedText(t.Context(), "text"); err == nil {
				t.Fatal("EmbedText accepted an invalid result")
			}
		})
	}

	client, _ := embeddingclient.New(embedding.ModelFunc(func(_ context.Context, request *embedding.Request) (*embedding.Response, error) {
		return responseFor(request.Texts), nil
	}))
	if _, err := client.EmbedTexts(t.Context(), []string{""}); err == nil {
		t.Fatal("EmbedTexts accepted empty text")
	}
	for _, docs := range [][]*document.Document{nil, {nil}, {{Media: nil}}} {
		if _, err := client.EmbedDocuments(t.Context(), docs); err == nil {
			t.Fatalf("EmbedDocuments accepted %#v", docs)
		}
	}
}
