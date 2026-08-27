package rag_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/rag"
)

var testQueryValueKey = rag.MustValueKey[string]("test value")

func TestNewQuery_RequiresText(t *testing.T) {
	if _, err := rag.NewQuery(""); !errors.Is(err, rag.ErrInvalidQuery) {
		t.Fatalf("NewQuery error = %v, want ErrInvalidQuery", err)
	}
}

func TestAugmentationOwnsValidatedCitationOrder(t *testing.T) {
	doc, _ := document.NewDocument("evidence", nil)
	citation, err := rag.NewCitation(1, candidate(doc))
	if err != nil {
		t.Fatal(err)
	}
	augmentation, err := rag.NewAugmentation("answer with [1]")
	if err != nil {
		t.Fatal(err)
	}
	augmentation, err = augmentation.WithCitations(rag.Citations{citation})
	if err != nil {
		t.Fatal(err)
	}
	citations := augmentation.Citations()
	citations[0].Number = 2
	if augmentation.Citations()[0].Number != 1 {
		t.Fatal("citation slice aliases caller mutation")
	}
	if _, err := rag.NewCitation(0, candidate(doc)); !errors.Is(err, rag.ErrInvalidAugmentation) {
		t.Fatalf("invalid citation number error = %v", err)
	}
	citation.Number = 2
	if _, err := augmentation.WithCitations(rag.Citations{citation}); !errors.Is(err, rag.ErrInvalidAugmentation) {
		t.Fatalf("non-consecutive citation error = %v", err)
	}
}

func TestQueryValidateRejectsZeroValue(t *testing.T) {
	if err := new(rag.Query).Validate(); !errors.Is(err, rag.ErrInvalidQuery) {
		t.Fatalf("zero-value Query.Validate error = %v", err)
	}
}

func TestQueryValue(t *testing.T) {
	q, _ := rag.NewQuery("hi")

	if value, found, err := q.Value(testQueryValueKey); err != nil || found || value != "" {
		t.Fatalf("Value(missing) = (%q, %v, %v)", value, found, err)
	}
	q, err := q.WithValue(testQueryValueKey, "v")
	if err != nil {
		t.Fatal(err)
	}
	if value, found, err := q.Value(testQueryValueKey); err != nil || !found || value != "v" {
		t.Fatalf("Value(test value) = (%q, %v, %v)", value, found, err)
	}
}

func TestWithValueReturnsIndependentQuery(t *testing.T) {
	a, _ := rag.NewQuery("hi")
	a, _ = a.WithValue(testQueryValueKey, "v")
	b, _ := a.WithValue(testQueryValueKey, "modified")

	if value, _, _ := a.Value(testQueryValueKey); value != "v" {
		t.Fatalf("update leaked into source query: a.k = %v", value)
	}
	if value, _, _ := b.Value(testQueryValueKey); value != "modified" {
		t.Fatalf("updated value = %v", value)
	}
}

func TestQueryValueKeyRejectsUntypedValuesAndIsolatesSameNameKeys(t *testing.T) {
	if _, err := rag.NewValueKey[any]("untyped"); !errors.Is(err, rag.ErrInvalidQueryValueKey) {
		t.Fatalf("NewValueKey[any] error = %v, want ErrInvalidQueryValueKey", err)
	}

	q, _ := rag.NewQuery("hi")
	q, _ = q.WithValue(testQueryValueKey, "v")
	independent := rag.MustValueKey[int](testQueryValueKey.Name())
	q, err := q.WithValue(independent, 1)
	if err != nil {
		t.Fatal(err)
	}
	if value, found, err := q.Value(independent); err != nil || !found || value != 1 {
		t.Fatalf("Value(independent key) = (%d, %v, %v)", value, found, err)
	}
	if value, found, err := q.Value(testQueryValueKey); err != nil || !found || value != "v" {
		t.Fatalf("Value(original key) = (%q, %v, %v)", value, found, err)
	}
	nilSliceKey := rag.MustValueKey[[]string]("nil slice")
	if _, err := q.WithValue(nilSliceKey, nil); !errors.Is(err, rag.ErrNilQueryValue) {
		t.Fatalf("WithValue nil error = %v, want ErrNilQueryValue", err)
	}
}

// fakeRetriever mocks Retriever for composition tests.
type fakeRetriever struct {
	docs rag.Candidates
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

func (f *fakeRetriever) Retrieve(_ context.Context, q rag.Query) (rag.Candidates, error) {
	f.hits++
	f.got = q.Text()
	if f.err != nil {
		return nil, f.err
	}
	return f.docs, nil
}

func TestRetrieveValidatesCandidates(t *testing.T) {
	retriever := &fakeRetriever{docs: rag.Candidates{{}}}
	query, _ := rag.NewQuery("query")

	if _, err := rag.Retrieve(t.Context(), retriever, query); !errors.Is(err, rag.ErrInvalidCandidate) {
		t.Fatalf("invalid candidate error = %v", err)
	}
}

func TestWithRefinersRejectsInvalidRetrieverOutputBeforeRefining(t *testing.T) {
	refinerCalled := false
	refined, err := rag.WithRefiners(
		&fakeRetriever{docs: rag.Candidates{{}}},
		rag.RefinerFunc(func(context.Context, rag.Query, rag.Candidates) (rag.Candidates, error) {
			refinerCalled = true
			return nil, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := refined.Retrieve(t.Context(), mustQuery(t, "query")); !errors.Is(err, rag.ErrInvalidCandidate) {
		t.Fatalf("invalid candidate error = %v", err)
	}
	if refinerCalled {
		t.Fatal("refiner received invalid candidates")
	}
}

func TestWithRefinersRejectsInvalidOutputBeforeNextRefiner(t *testing.T) {
	secondCalled := false
	refined, err := rag.WithRefiners(
		&fakeRetriever{},
		rag.RefinerFunc(func(context.Context, rag.Query, rag.Candidates) (rag.Candidates, error) {
			return rag.Candidates{{}}, nil
		}),
		rag.RefinerFunc(func(context.Context, rag.Query, rag.Candidates) (rag.Candidates, error) {
			secondCalled = true
			return nil, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := refined.Retrieve(t.Context(), mustQuery(t, "query")); !errors.Is(err, rag.ErrInvalidCandidate) {
		t.Fatalf("invalid candidate error = %v", err)
	}
	if secondCalled {
		t.Fatal("next refiner received invalid candidates")
	}
}

// fakeTransformer mocks Transformer.
type fakeTransformer struct {
	suffix string
	err    error
}

func (f *fakeTransformer) Transform(_ context.Context, q rag.Query) (rag.Query, error) {
	if f.err != nil {
		return rag.Query{}, f.err
	}
	return q.WithText(q.Text() + f.suffix)
}

func TestWithTransformersFeedsTransformedQueryToRetriever(t *testing.T) {
	doc, _ := document.NewDocument("retrieved-doc", nil)
	retriever := &fakeRetriever{docs: rag.Candidates{candidate(doc)}}

	r, err := rag.WithTransformers(retriever, &fakeTransformer{suffix: "?"})
	if err != nil {
		t.Fatal(err)
	}
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

	r, err := rag.WithTransformers(retriever, &fakeTransformer{err: want})
	if err != nil {
		t.Fatal(err)
	}
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
	r1 := &fakeRetriever{docs: rag.Candidates{candidate(docA)}}
	r2 := &fakeRetriever{docs: rag.Candidates{candidate(docB)}}

	combined, err := rag.Parallel(r1, r2)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := combined.Retrieve(t.Context(), mustQuery(t, "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2 (union of r1, r2)", len(docs))
	}
}

func TestParallelRejectsPartialResults(t *testing.T) {
	docA, _ := document.NewDocument("a", nil)
	r1 := &fakeRetriever{docs: rag.Candidates{candidate(docA)}}
	r2 := &fakeRetriever{err: errors.New("retriever 2 broken")}

	combined, err := rag.Parallel(r1, r2)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := combined.Retrieve(t.Context(), mustQuery(t, "hi"))
	if err == nil {
		t.Fatal("partial retrieval must fail")
	}
	if docs != nil {
		t.Fatalf("docs = %#v, want nil on partial failure", docs)
	}
}

func TestParallelAcceptsEmptySuccessfulResults(t *testing.T) {
	combined, err := rag.Parallel(&fakeRetriever{}, &fakeRetriever{})
	if err != nil {
		t.Fatal(err)
	}

	docs, err := combined.Retrieve(t.Context(), mustQuery(t, "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if docs != nil {
		t.Fatalf("docs = %#v, want nil", docs)
	}
}

func TestParallelOwnsConfigurationAndOrdersFailuresByDeclaration(t *testing.T) {
	firstFailure := errors.New("first failure")
	secondFailure := errors.New("second failure")
	secondFinished := make(chan struct{})
	first := rag.RetrieverFunc(func(context.Context, rag.Query) (rag.Candidates, error) {
		<-secondFinished
		return nil, firstFailure
	})
	second := rag.RetrieverFunc(func(context.Context, rag.Query) (rag.Candidates, error) {
		close(secondFinished)
		return nil, secondFailure
	})
	retrievers := []rag.Retriever{first, second}
	combined, err := rag.Parallel(retrievers...)
	if err != nil {
		t.Fatal(err)
	}
	retrievers[0] = &fakeRetriever{}

	_, err = combined.Retrieve(t.Context(), mustQuery(t, "hi"))
	if !errors.Is(err, firstFailure) || !errors.Is(err, secondFailure) {
		t.Fatalf("Parallel error = %v, want both failures", err)
	}
	message := err.Error()
	if strings.Index(message, firstFailure.Error()) > strings.Index(message, secondFailure.Error()) {
		t.Fatalf("failure order followed completion order: %v", err)
	}
}

func TestCombinatorsRejectNilCapabilitiesAtConstruction(t *testing.T) {
	var transformer *fakeTransformer
	if _, err := rag.WithTransformers(&fakeRetriever{}, transformer); !errors.Is(err, rag.ErrNilTransformer) {
		t.Fatalf("WithTransformers error = %v, want ErrNilTransformer", err)
	}
	if _, err := rag.WithExpander(&fakeRetriever{}, nil); !errors.Is(err, rag.ErrNilExpander) {
		t.Fatalf("WithExpander error = %v, want ErrNilExpander", err)
	}
	if _, err := rag.WithRefiners(&fakeRetriever{}, nil); !errors.Is(err, rag.ErrNilRefiner) {
		t.Fatalf("WithRefiners error = %v, want ErrNilRefiner", err)
	}
	if _, err := rag.Parallel(&fakeRetriever{}, nil); !errors.Is(err, rag.ErrNilRetriever) {
		t.Fatalf("Parallel error = %v, want ErrNilRetriever", err)
	}
}

func TestWithExpanderRejectsEmptyExpansion(t *testing.T) {
	expanded, err := rag.WithExpander(
		&fakeRetriever{},
		rag.ExpanderFunc(func(context.Context, rag.Query) ([]rag.Query, error) {
			return nil, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = expanded.Retrieve(t.Context(), mustQuery(t, "hi"))
	if !errors.Is(err, rag.ErrEmptyExpansion) {
		t.Fatalf("Retrieve error = %v, want ErrEmptyExpansion", err)
	}
}

func TestIdentityDefaults(t *testing.T) {
	q, _ := rag.NewQuery("hi")

	if got, _ := rag.IdentityExpander().Expand(t.Context(), q); len(got) != 1 || got[0].Text() != q.Text() {
		t.Fatal("Expand should pass through")
	}
	if got, _ := rag.IdentityTransformer().Transform(t.Context(), q); got.Text() != q.Text() {
		t.Fatal("Transform should pass through")
	}
	if got, _ := rag.IdentityAugmenter().Augment(t.Context(), q, nil); got.Text() != q.Text() {
		t.Fatal("Augment should preserve query text")
	}
	if got, _ := rag.NopRetriever().Retrieve(t.Context(), q); got != nil {
		t.Fatal("Retrieve should return nil")
	}
	if got, _ := rag.IdentityRefiner().Refine(t.Context(), q, nil); got != nil {
		t.Fatal("Refine should pass through nil")
	}
}

func mustQuery(t *testing.T, text string) rag.Query {
	t.Helper()
	q, err := rag.NewQuery(text)
	if err != nil {
		t.Fatal(err)
	}
	return q
}
