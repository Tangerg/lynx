package rag_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/rag"
)

func TestDedupDropsDuplicateIDs(t *testing.T) {
	r := rag.Dedup()

	a, _ := document.NewDocument("a", nil)
	a.ID = "1"
	b, _ := document.NewDocument("b", nil)
	b.ID = "2"
	dup, _ := document.NewDocument("a-dup", nil)
	dup.ID = "1"

	got, err := r.Refine(t.Context(), nil, []rag.Candidate{candidate(a), candidate(b), candidate(dup)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d docs, want 2", len(got))
	}
	if got[0].Document.ID != "1" || got[1].Document.ID != "2" {
		t.Fatalf("first-occurrence order broken: %s,%s", got[0].Document.ID, got[1].Document.ID)
	}
}

func TestDedupHonorsContextCancel(t *testing.T) {
	r := rag.Dedup()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := r.Refine(ctx, nil, nil); err == nil {
		t.Fatal("canceled ctx must error")
	}
}

func TestTopKSortsAndCaps(t *testing.T) {
	r, err := rag.TopK(2)
	if err != nil {
		t.Fatal(err)
	}

	aDoc, _ := document.NewDocument("a", nil)
	bDoc, _ := document.NewDocument("b", nil)
	cDoc, _ := document.NewDocument("c", nil)
	a := candidate(aDoc, 0.3)
	b := candidate(bDoc, 0.9)
	c := candidate(cDoc, 0.5)

	got, err := r.Refine(t.Context(), nil, []rag.Candidate{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d docs, want 2 (capped to topK)", len(got))
	}
	if got[0].Score != 0.9 || got[1].Score != 0.5 {
		t.Fatalf("sort order broken: %v, %v", got[0].Score, got[1].Score)
	}
}

func TestTopKRejectsNonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		if _, err := rag.TopK(limit); err == nil {
			t.Fatalf("TopK(%d) succeeded, want error", limit)
		}
	}
}

func TestTopKDoesNotMutateInput(t *testing.T) {
	r, err := rag.TopK(10)
	if err != nil {
		t.Fatal(err)
	}

	aDoc, _ := document.NewDocument("a", nil)
	bDoc, _ := document.NewDocument("b", nil)
	a := candidate(aDoc, 0.1)
	b := candidate(bDoc, 0.9)
	in := []rag.Candidate{a, b}

	_, _ = r.Refine(t.Context(), nil, in)

	if in[0].Score != 0.1 || in[1].Score != 0.9 {
		t.Fatalf("input mutated: %v %v", in[0].Score, in[1].Score)
	}
}
