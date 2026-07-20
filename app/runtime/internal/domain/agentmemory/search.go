package agentmemory

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"
	"unicode"
)

// Embedder turns text into vectors for semantic memory search. It is satisfied
// by the same embedding client @codebase uses; the [Searcher] treats it as
// optional and best-effort.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	ID() string
}

// itemSearchStore is the slice of [Store] the searcher needs: the fetch of a
// project's items (with embeddings) it ranks in-process.
type itemSearchStore interface {
	ItemsForSearch(ctx context.Context, scope Scope, project string) ([]Item, error)
}

// Searcher ranks a project's memory items against a query, fusing a keyword
// signal with cosine similarity over item embeddings. The keyword signal is
// always available; the vector half is added only when an embedder is
// configured, and a configured embedder that fails degrades to keyword-only
// rather than failing the search. The item corpus is small, so both signals are
// computed in-process over the fetched items.
type Searcher struct {
	store           itemSearchStore
	resolveEmbedder func(context.Context) (Embedder, error)
}

// NewSearcher builds a searcher over store. resolveEmbedder may be nil (keyword
// only) or return an error when no embedding model is configured.
func NewSearcher(store itemSearchStore, resolveEmbedder func(context.Context) (Embedder, error)) *Searcher {
	return &Searcher{store: store, resolveEmbedder: resolveEmbedder}
}

// searchOverfetch widens each signal's candidate list before fusion so a
// keyword-only or vector-only match still competes for the final top-k.
const searchOverfetch = 3

// Search returns up to topK items most relevant to query, fusing keyword and
// vector rankings by reciprocal rank.
func (s *Searcher) Search(ctx context.Context, scope Scope, project, query string, topK int) ([]Item, error) {
	if s == nil || s.store == nil || topK <= 0 {
		return nil, nil
	}
	items, err := s.store.ItemsForSearch(ctx, scope, project)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	keyword := keywordRanked(query, items, topK*searchOverfetch)
	vector := s.vectorRanked(ctx, query, items, topK*searchOverfetch)
	return fuseByRank(keyword, vector, topK), nil
}

// vectorRanked is best-effort: no embedder, an unconfigured role, or an embed
// failure yields no vector signal (keyword still stands), never an error.
func (s *Searcher) vectorRanked(ctx context.Context, query string, items []Item, limit int) []Item {
	if s.resolveEmbedder == nil {
		return nil
	}
	embedder, err := s.resolveEmbedder(ctx)
	if err != nil || embedder == nil {
		return nil
	}
	vecs, err := embedder.Embed(ctx, []string{query})
	if err != nil || len(vecs) != 1 {
		return nil
	}
	return topKByCosine(vecs[0], items, limit)
}

// keywordRanked scores items by how many distinct query terms appear in their
// content and returns the best matches, highest first. Items with no overlap
// are dropped so the keyword signal stays precise.
func keywordRanked(query string, items []Item, limit int) []Item {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}
	type scored struct {
		item  Item
		score int
	}
	ranked := make([]scored, 0, len(items))
	for _, item := range items {
		content := strings.ToLower(item.Content)
		hits := 0
		for _, term := range terms {
			if strings.Contains(content, term) {
				hits++
			}
		}
		if hits == 0 {
			continue
		}
		ranked = append(ranked, scored{item: item, score: hits})
	}
	slices.SortStableFunc(ranked, func(a, b scored) int { return cmp.Compare(b.score, a.score) })
	return capItems(ranked, limit, func(s scored) Item { return s.item })
}

func topKByCosine(query []float32, items []Item, limit int) []Item {
	qn := vectorNorm(query)
	if qn == 0 {
		return nil
	}
	type scored struct {
		item  Item
		score float64
	}
	ranked := make([]scored, 0, len(items))
	for _, item := range items {
		score := cosineSim(query, qn, item.Embedding)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, scored{item: item, score: score})
	}
	slices.SortStableFunc(ranked, func(a, b scored) int { return cmp.Compare(b.score, a.score) })
	return capItems(ranked, limit, func(s scored) Item { return s.item })
}

func capItems[T any](ranked []T, limit int, pick func(T) Item) []Item {
	out := make([]Item, 0, min(limit, len(ranked)))
	for i := 0; i < len(ranked) && i < limit; i++ {
		out = append(out, pick(ranked[i]))
	}
	return out
}

// fuseByRank merges two ranked lists by reciprocal-rank fusion and returns the
// top-k distinct items (by id). An item ranked by both signals accumulates both
// contributions, so agreement wins.
func fuseByRank(keyword, vector []Item, topK int) []Item {
	const rrfK = 60.0
	score := make(map[string]float64)
	item := make(map[string]Item)
	for _, list := range [][]Item{keyword, vector} {
		for rank, it := range list {
			score[it.ID] += 1.0 / (rrfK + float64(rank+1))
			item[it.ID] = it
		}
	}
	ids := make([]string, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		if d := cmp.Compare(score[b], score[a]); d != 0 {
			return d
		}
		return cmp.Compare(a, b) // deterministic tiebreak
	})
	out := make([]Item, 0, min(topK, len(ids)))
	for _, id := range ids {
		if len(out) >= topK {
			break
		}
		out = append(out, item[id])
	}
	return out
}

func vectorNorm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}

// cosineSim returns the cosine similarity of v against query (whose precomputed
// norm is qn). 0 for a dimension mismatch or a zero vector.
func cosineSim(query []float32, qn float64, v []float32) float64 {
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

// tokenize lower-cases and splits text into distinct alphanumeric terms of at
// least two runes, dropping punctuation and one-letter noise.
func tokenize(text string) []string {
	seen := make(map[string]struct{})
	var terms []string
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if len([]rune(field)) < 2 {
			continue
		}
		if _, dup := seen[field]; dup {
			continue
		}
		seen[field] = struct{}{}
		terms = append(terms, field)
	}
	return terms
}
