package rag_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/rag"
)

func TestNewQuery_RequiresText(t *testing.T) {
	if _, err := rag.NewQuery(""); err == nil {
		t.Fatal("empty text must error")
	}
	if !strings.Contains(mustErr(rag.NewQuery("")).Error(), "rag.NewQuery") {
		t.Fatal("error must include package context")
	}
}

func TestQuery_GetSetExtra(t *testing.T) {
	q, _ := rag.NewQuery("hi")

	if v, ok := q.Get("missing"); ok || v != nil {
		t.Fatalf("Get(missing) = (%v,%v)", v, ok)
	}
	q.Set("k", "v")
	if v, _ := q.Get("k"); v != "v" {
		t.Fatalf("Get(k) = %v", v)
	}
}

func TestQuery_Clone_Independence(t *testing.T) {
	a, _ := rag.NewQuery("hi")
	a.Set("k", "v")

	b := a.Clone()
	b.Set("k", "modified")

	if v, _ := a.Get("k"); v != "v" {
		t.Fatalf("clone leaked: a.k = %v", v)
	}
}

// fakeRetriever mocks Retriever for composition tests.
type fakeRetriever struct {
	docs []*document.Document
	err  error
	hits int
	got  string
}

func (r *fakeRetriever) Retrieve(_ context.Context, q *rag.Query) ([]*document.Document, error) {
	r.hits++
	if q != nil {
		r.got = q.Text
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
	out := q.Clone()
	out.Text += t.suffix
	return out, nil
}

func TestWithTransformersFeedsTransformedQueryToRetriever(t *testing.T) {
	doc, _ := document.NewDocument("retrieved-doc", nil)
	retriever := &fakeRetriever{docs: []*document.Document{doc}}

	r := rag.WithTransformers(retriever, &fakeTransformer{suffix: "?"})
	docs, err := r.Retrieve(context.Background(), mustQuery(t, "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if retriever.got != "hi?" {
		t.Fatalf("retriever query = %q, want hi?", retriever.got)
	}
	if len(docs) != 1 || docs[0] != doc {
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
	if _, err := r.Retrieve(context.Background(), mustQuery(t, "hi")); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	if retriever.hits != 0 {
		t.Fatal("retriever ran despite transformer failure")
	}
}

func TestMultiUnionsResults(t *testing.T) {
	docA, _ := document.NewDocument("a", nil)
	docB, _ := document.NewDocument("b", nil)
	r1 := &fakeRetriever{docs: []*document.Document{docA}}
	r2 := &fakeRetriever{docs: []*document.Document{docB}}

	docs, err := rag.Multi(r1, r2).Retrieve(context.Background(), mustQuery(t, "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2 (union of r1, r2)", len(docs))
	}
}

func TestMultiPartialFailureReturnsAvailableDocs(t *testing.T) {
	docA, _ := document.NewDocument("a", nil)
	r1 := &fakeRetriever{docs: []*document.Document{docA}}
	r2 := &fakeRetriever{err: errors.New("retriever 2 broken")}

	docs, err := rag.Multi(r1, r2).Retrieve(context.Background(), mustQuery(t, "hi"))
	if err != nil {
		t.Fatalf("partial failure should not fail the whole retrieval: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1 (the surviving retriever)", len(docs))
	}
}

func TestIdentityDefaults(t *testing.T) {
	q, _ := rag.NewQuery("hi")

	if got, _ := rag.IdentityExpander().Expand(context.Background(), q); len(got) != 1 || got[0] != q {
		t.Fatal("Expand should pass through")
	}
	if got, _ := rag.IdentityTransformer().Transform(context.Background(), q); got != q {
		t.Fatal("Transform should pass through")
	}
	if got, _ := rag.IdentityAugmenter().Augment(context.Background(), q, nil); got != q {
		t.Fatal("Augment should pass through")
	}
	if got, _ := rag.NoopRetriever().Retrieve(context.Background(), q); got != nil {
		t.Fatal("Retrieve should return nil")
	}
	if got, _ := rag.IdentityRefiner().Refine(context.Background(), q, nil); got != nil {
		t.Fatal("Refine should pass through nil")
	}
}

func mustErr[T any](_ T, err error) error {
	if err == nil {
		panic("expected an error")
	}
	return err
}

func mustQuery(t *testing.T, text string) *rag.Query {
	t.Helper()
	q, err := rag.NewQuery(text)
	if err != nil {
		t.Fatal(err)
	}
	return q
}
