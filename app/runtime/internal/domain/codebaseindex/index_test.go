package codebaseindex

import "testing"

func TestRankReturnsHighestCosineScores(t *testing.T) {
	chunks := []Chunk{
		{Path: "orthogonal.go", Embedding: []float32{0, 1}},
		{Path: "matching.go", Embedding: []float32{1, 0}},
		{Path: "opposite.go", Embedding: []float32{-1, 0}},
	}

	hits := Rank([]float32{1, 0}, chunks, 2)
	if len(hits) != 2 || hits[0].Path != "matching.go" || hits[0].Score != 1 ||
		hits[1].Path != "orthogonal.go" || hits[1].Score != 0 {
		t.Fatalf("Rank() = %#v, want matching then orthogonal", hits)
	}
}

func TestRankRejectsNonPositiveLimitAndUnusableQuery(t *testing.T) {
	chunks := []Chunk{{Path: "file.go", Embedding: []float32{1}}}
	for _, test := range []struct {
		name  string
		query []float32
		limit int
	}{
		{name: "zero limit", query: []float32{1}, limit: 0},
		{name: "negative limit", query: []float32{1}, limit: -1},
		{name: "zero query", query: []float32{0}, limit: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if hits := Rank(test.query, chunks, test.limit); hits != nil {
				t.Fatalf("Rank() = %#v, want nil", hits)
			}
		})
	}
}
