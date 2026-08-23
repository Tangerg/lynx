package agentmemory

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"unicode"
)

const searchOverfetch = 3

type Candidate struct {
	Item   Item
	Vector []float64
}

// Rank combines lexical and optional semantic relevance. Empty vectors select
// the deterministic lexical path, so recall remains useful without a model.
func Rank(
	query string,
	queryVector []float64,
	candidates []Candidate,
	limit int,
) []Item {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	keyword := rankKeywords(query, candidates, limit*searchOverfetch)
	semantic := rankVectors(queryVector, candidates, limit*searchOverfetch)
	return fuse(keyword, semantic, limit)
}

func rankKeywords(query string, candidates []Candidate, limit int) []Item {
	terms := tokens(query)
	if len(terms) == 0 {
		return nil
	}
	type scored struct {
		item  Item
		score int
	}
	values := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		content := strings.ToLower(candidate.Item.Content)
		score := 0
		for _, term := range terms {
			if strings.Contains(content, term) {
				score++
			}
		}
		if score > 0 {
			values = append(values, scored{item: candidate.Item, score: score})
		}
	}
	slices.SortStableFunc(values, func(left, right scored) int {
		if order := cmp.Compare(right.score, left.score); order != 0 {
			return order
		}
		return compareRecall(left.item, right.item)
	})
	return take(values, limit, func(value scored) Item { return value.item })
}

func rankVectors(query []float64, candidates []Candidate, limit int) []Item {
	queryNorm := norm(query)
	if queryNorm == 0 {
		return nil
	}
	type scored struct {
		item  Item
		score float64
	}
	values := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		score := cosine(query, queryNorm, candidate.Vector)
		if score > 0 {
			values = append(values, scored{item: candidate.Item, score: score})
		}
	}
	slices.SortStableFunc(values, func(left, right scored) int {
		if order := cmp.Compare(right.score, left.score); order != 0 {
			return order
		}
		return compareRecall(left.item, right.item)
	})
	return take(values, limit, func(value scored) Item { return value.item })
}

func fuse(keyword []Item, semantic []Item, limit int) []Item {
	const reciprocalRankOffset = 60.0
	scores := make(map[string]float64, len(keyword)+len(semantic))
	items := make(map[string]Item, len(keyword)+len(semantic))
	for _, ranked := range [][]Item{keyword, semantic} {
		for index, item := range ranked {
			scores[item.ID] += 1 / (reciprocalRankOffset + float64(index+1))
			items[item.ID] = item
		}
	}
	ids := make([]string, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(left, right string) int {
		if order := cmp.Compare(scores[right], scores[left]); order != 0 {
			return order
		}
		return compareRecall(items[left], items[right])
	})
	result := make([]Item, 0, min(limit, len(ids)))
	for _, id := range ids {
		if len(result) == limit {
			break
		}
		result = append(result, items[id])
	}
	return result
}

func compareRecall(left Item, right Item) int {
	if left.Pinned != right.Pinned {
		if left.Pinned {
			return -1
		}
		return 1
	}
	if order := right.UpdatedAt.Compare(left.UpdatedAt); order != 0 {
		return order
	}
	return cmp.Compare(left.ID, right.ID)
}

func take[T any](values []T, limit int, selectItem func(T) Item) []Item {
	result := make([]Item, 0, min(limit, len(values)))
	for index := 0; index < len(values) && index < limit; index++ {
		result = append(result, selectItem(values[index]))
	}
	return result
}

func tokens(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, field := range strings.FieldsFunc(text, func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsNumber(value)
	}) {
		if len([]rune(field)) < 2 {
			continue
		}
		if _, duplicate := seen[field]; duplicate {
			continue
		}
		seen[field] = struct{}{}
		values = append(values, field)
	}
	return values
}

func norm(vector []float64) float64 {
	value := 0.0
	for _, component := range vector {
		if math.IsNaN(component) || math.IsInf(component, 0) {
			return 0
		}
		value += component * component
	}
	return math.Sqrt(value)
}

func cosine(left []float64, leftNorm float64, right []float64) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	dot := 0.0
	rightSquare := 0.0
	for index := range left {
		if math.IsNaN(right[index]) || math.IsInf(right[index], 0) {
			return 0
		}
		dot += left[index] * right[index]
		rightSquare += right[index] * right[index]
	}
	rightNorm := math.Sqrt(rightSquare)
	if rightNorm == 0 {
		return 0
	}
	return dot / (leftNorm * rightNorm)
}
