package codebaseindex

import (
	"cmp"
	"math"
	"slices"
)

// topKHits scores every chunk against the query vector by cosine similarity and
// returns the k highest, descending.
func topKHits(query []float32, chunks []Chunk, k int) []Hit {
	qn := norm(query)
	if qn == 0 {
		return nil
	}
	hits := make([]Hit, 0, len(chunks))
	for i := range chunks {
		hits = append(hits, Hit{
			Path:      chunks[i].Path,
			StartLine: chunks[i].StartLine,
			EndLine:   chunks[i].EndLine,
			Text:      chunks[i].Text,
			Score:     cosine(query, qn, chunks[i].Embedding),
		})
	}
	slices.SortFunc(hits, func(a, b Hit) int { return cmp.Compare(b.Score, a.Score) })
	return hits[:min(k, len(hits))]
}

func norm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

// cosine returns the cosine similarity of v against query (whose precomputed
// norm is qn). 0 for a dimension mismatch or a zero vector.
func cosine(query []float32, qn float64, v []float32) float64 {
	if len(v) != len(query) {
		return 0
	}
	var dot, vn float64
	for i := range query {
		dot += float64(query[i]) * float64(v[i])
		vn += float64(v[i]) * float64(v[i])
	}
	vn = math.Sqrt(vn)
	if vn == 0 {
		return 0
	}
	return dot / (qn * vn)
}
