package rag_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/rag"
)

func TestDeduplicationRefiner_DropsDuplicateIDs(t *testing.T) {
	r := rag.NewDeduplicationRefiner()

	a, _ := document.NewDocument("a", nil)
	a.ID = "1"
	b, _ := document.NewDocument("b", nil)
	b.ID = "2"
	dup, _ := document.NewDocument("a-dup", nil)
	dup.ID = "1"

	got, err := r.Refine(context.Background(), nil, []*document.Document{a, b, dup})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d docs, want 2", len(got))
	}
	if got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("first-occurrence order broken: %s,%s", got[0].ID, got[1].ID)
	}
}

func TestDeduplicationRefiner_HonorsContextCancel(t *testing.T) {
	r := rag.NewDeduplicationRefiner()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := r.Refine(ctx, nil, nil); err == nil {
		t.Fatal("cancelled ctx must error")
	}
}

func TestRankRefiner_SortsAndCaps(t *testing.T) {
	r := rag.NewRankRefiner(2)

	a, _ := document.NewDocument("a", nil)
	a.Score = 0.3
	b, _ := document.NewDocument("b", nil)
	b.Score = 0.9
	c, _ := document.NewDocument("c", nil)
	c.Score = 0.5

	got, err := r.Refine(context.Background(), nil, []*document.Document{a, b, c})
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

func TestNewRankRefiner_NormalizesNonPositiveTopK(t *testing.T) {
	// topK 0 / negative should fall back to 1, not panic / not return empty.
	r := rag.NewRankRefiner(0)
	a, _ := document.NewDocument("a", nil)
	got, err := r.Refine(context.Background(), nil, []*document.Document{a})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

func TestRankRefiner_DoesNotMutateInput(t *testing.T) {
	r := rag.NewRankRefiner(10)

	a, _ := document.NewDocument("a", nil)
	a.Score = 0.1
	b, _ := document.NewDocument("b", nil)
	b.Score = 0.9
	in := []*document.Document{a, b}

	_, _ = r.Refine(context.Background(), nil, in)

	if in[0].Score != 0.1 || in[1].Score != 0.9 {
		t.Fatalf("input mutated: %v %v", in[0].Score, in[1].Score)
	}
}
