package rag_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/document"
	corererank "github.com/Tangerg/scope/core/rerank"
	"github.com/Tangerg/scope/rag"
)

func TestRerankerMapsModelIndicesToOwnedCandidates(t *testing.T) {
	var seen *corererank.Request
	model := corererank.ModelFunc(func(_ context.Context, request *corererank.Request) (*corererank.Response, error) {
		seen = request
		return &corererank.Response{Results: []*corererank.Result{
			{Index: 1, Score: 0.9},
			{Index: 0, Score: 0.4},
		}}, nil
	})
	reranker, err := rag.NewReranker(rag.RerankerConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	first := identifiedDocument(t, "first", "first content")
	second := identifiedDocument(t, "second", "second content")
	input := rag.Candidates{candidate(first, 100), candidate(second, -10)}
	got, err := reranker.Refine(t.Context(), mustQuery(t, "query"), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Document.ID != "second" || got[0].Score != 0.9 || got[1].Document.ID != "first" || got[1].Score != 0.4 {
		t.Fatalf("reranked candidates = %#v", got)
	}
	if got[0].Document == second || got[1].Document == first {
		t.Fatal("reranker returned caller-owned documents")
	}
	if seen.Query != "query" || len(seen.Documents) != 2 || seen.Documents[0] != "first content" {
		t.Fatalf("model request = %#v", seen)
	}
}

func TestRerankerTopKAndResponseContract(t *testing.T) {
	model := corererank.ModelFunc(func(_ context.Context, request *corererank.Request) (*corererank.Response, error) {
		if request.Options.TopK != 1 {
			t.Fatalf("TopK = %d, want 1", request.Options.TopK)
		}
		return &corererank.Response{Results: []*corererank.Result{{Index: 1, Score: 0.8}}}, nil
	})
	reranker, err := rag.NewReranker(rag.RerankerConfig{Model: model, TopK: 1})
	if err != nil {
		t.Fatal(err)
	}
	candidates := rag.Candidates{
		candidate(identifiedDocument(t, "first", "first")),
		candidate(identifiedDocument(t, "second", "second")),
	}
	got, err := reranker.Refine(t.Context(), mustQuery(t, "query"), candidates)
	if err != nil || len(got) != 1 || got[0].Document.ID != "second" {
		t.Fatalf("Refine = %#v, %v", got, err)
	}

	invalid, err := rag.NewReranker(rag.RerankerConfig{Model: corererank.ModelFunc(func(context.Context, *corererank.Request) (*corererank.Response, error) {
		return &corererank.Response{Results: []*corererank.Result{{Index: 2, Score: 0.8}}}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invalid.Refine(t.Context(), mustQuery(t, "query"), candidates); !errors.Is(err, rag.ErrInvalidReranking) {
		t.Fatalf("invalid response error = %v", err)
	}
}

func TestRerankerValidatesConstructionAndFormatting(t *testing.T) {
	if _, err := rag.NewReranker(rag.RerankerConfig{}); !errors.Is(err, rag.ErrNilRerankModel) {
		t.Fatalf("missing model error = %v", err)
	}
	if _, err := rag.NewReranker(rag.RerankerConfig{Model: corererank.ModelFunc(func(context.Context, *corererank.Request) (*corererank.Response, error) { return nil, nil }), TopK: -1}); !errors.Is(err, rag.ErrInvalidReranking) {
		t.Fatalf("negative TopK error = %v", err)
	}
	formatter := rag.DocumentFormatterFunc(func(*document.Document) (string, error) { return " ", nil })
	reranker, err := rag.NewReranker(rag.RerankerConfig{
		Model:     corererank.ModelFunc(func(context.Context, *corererank.Request) (*corererank.Response, error) { return nil, nil }),
		Formatter: formatter,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reranker.Refine(t.Context(), mustQuery(t, "query"), rag.Candidates{candidate(identifiedDocument(t, "id", "text"))}); !errors.Is(err, rag.ErrInvalidReranking) {
		t.Fatalf("blank formatting error = %v", err)
	}
}
