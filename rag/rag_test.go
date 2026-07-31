package rag_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/rag"
)

func TestNewQuery_RequiresText(t *testing.T) {
	if _, err := rag.NewQuery(""); !errors.Is(err, rag.ErrInvalidQuery) {
		t.Fatalf("NewQuery error = %v, want ErrInvalidQuery", err)
	}
}

func TestQueryValidateRejectsZeroValue(t *testing.T) {
	if err := new(rag.Query).Validate(); !errors.Is(err, rag.ErrInvalidQuery) {
		t.Fatalf("zero-value Query.Validate error = %v", err)
	}
}

func TestQuery_Value(t *testing.T) {
	q, _ := rag.NewQuery("hi")

	if value, found := q.Value("missing"); found || value != nil {
		t.Fatalf("Value(missing) = (%v,%v)", value, found)
	}
	q, err := q.WithValue("k", "v")
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := q.Value("k"); value != "v" {
		t.Fatalf("Value(k) = %v", value)
	}
}

func TestQuery_WithValueReturnsIndependentQuery(t *testing.T) {
	a, _ := rag.NewQuery("hi")
	a, _ = a.WithValue("k", "v")
	b, _ := a.WithValue("k", "modified")

	if value, _ := a.Value("k"); value != "v" {
		t.Fatalf("update leaked into source query: a.k = %v", value)
	}
	if value, _ := b.Value("k"); value != "modified" {
		t.Fatalf("updated value = %v", value)
	}
}

// fakeRetriever mocks Retriever for composition tests.
type fakeRetriever struct {
	docs []rag.Candidate
	err  error
	hits int
	got  string
}

func candidate(doc *document.Document, score ...float64) rag.Candidate {
	var value float64
	if len(score) > 0 {
		value = score[0]
	}
	return rag.Candidate{Document: doc, Score: value}
}

func (r *fakeRetriever) Retrieve(_ context.Context, q *rag.Query) ([]rag.Candidate, error) {
	r.hits++
	if q != nil {
		r.got = q.Text()
	}
	if r.err != nil {
		return nil, r.err
	}
	return r.docs, nil
}

// fakeTransformer mocks Transformer.
type fakeTransformer struct {
	suffix string
	err    error
}

func (t *fakeTransformer) Transform(_ context.Context, q *rag.Query) (*rag.Query, error) {
	if t.err != nil {
		return nil, t.err
	}
	return q.WithText(q.Text() + t.suffix)
}

func TestWithTransformersFeedsTransformedQueryToRetriever(t *testing.T) {
	doc, _ := document.NewDocument("retrieved-doc", nil)
	retriever := &fakeRetriever{docs: []rag.Candidate{candidate(doc)}}

	r := rag.WithTransformers(retriever, &fakeTransformer{suffix: "?"})
	docs, err := r.Retrieve(t.Context(), mustQuery(t, "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if retriever.got != "hi?" {
		t.Fatalf("retriever query = %q, want hi?", retriever.got)
	}
	if len(docs) != 1 || docs[0].Document != doc {
		t.Fatalf("docs = %v", docs)
	}
	if retriever.hits != 1 {
		t.Fatalf("retriever hits = %d, want 1", retriever.hits)
	}
}

func TestWithTransformersErrorShortCircuits(t *testing.T) {
	want := errors.New("boom")
	retriever := &fakeRetriever{}

	r := rag.WithTransformers(retriever, &fakeTransformer{err: want})
	if _, err := r.Retrieve(t.Context(), mustQuery(t, "hi")); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	if retriever.hits != 0 {
		t.Fatal("retriever ran despite transformer failure")
	}
}

func TestParallelUnionsResults(t *testing.T) {
	docA, _ := document.NewDocument("a", nil)
	docB, _ := document.NewDocument("b", nil)
	r1 := &fakeRetriever{docs: []rag.Candidate{candidate(docA)}}
	r2 := &fakeRetriever{docs: []rag.Candidate{candidate(docB)}}

	docs, err := rag.Parallel(r1, r2).Retrieve(t.Context(), mustQuery(t, "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2 (union of r1, r2)", len(docs))
	}
}

func TestParallelPartialFailureReturnsAvailableDocs(t *testing.T) {
	docA, _ := document.NewDocument("a", nil)
	r1 := &fakeRetriever{docs: []rag.Candidate{candidate(docA)}}
	r2 := &fakeRetriever{err: errors.New("retriever 2 broken")}

	docs, err := rag.Parallel(r1, r2).Retrieve(t.Context(), mustQuery(t, "hi"))
	if err != nil {
		t.Fatalf("partial failure should not fail the whole retrieval: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1 (the surviving retriever)", len(docs))
	}
}

func TestParallelTreatsEmptySuccessfulResultAsSuccess(t *testing.T) {
	success := &fakeRetriever{}
	failure := &fakeRetriever{err: errors.New("broken")}

	docs, err := rag.Parallel(success, failure).Retrieve(t.Context(), mustQuery(t, "hi"))
	if err != nil {
		t.Fatalf("empty successful result was discarded: %v", err)
	}
	if docs != nil {
		t.Fatalf("docs = %#v, want nil", docs)
	}
}

func TestIdentityDefaults(t *testing.T) {
	q, _ := rag.NewQuery("hi")

	if got, _ := rag.IdentityExpander().Expand(t.Context(), q); len(got) != 1 || got[0] != q {
		t.Fatal("Expand should pass through")
	}
	if got, _ := rag.IdentityTransformer().Transform(t.Context(), q); got != q {
		t.Fatal("Transform should pass through")
	}
	if got, _ := rag.IdentityAugmenter().Augment(t.Context(), q, nil); got != q {
		t.Fatal("Augment should pass through")
	}
	if got, _ := rag.NopRetriever().Retrieve(t.Context(), q); got != nil {
		t.Fatal("Retrieve should return nil")
	}
	if got, _ := rag.IdentityRefiner().Refine(t.Context(), q, nil); got != nil {
		t.Fatal("Refine should pass through nil")
	}
}

func mustQuery(t *testing.T, text string) *rag.Query {
	t.Helper()
	q, err := rag.NewQuery(text)
	if err != nil {
		t.Fatal(err)
	}
	return q
}
