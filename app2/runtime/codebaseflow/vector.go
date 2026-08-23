package codebaseflow

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/embedding"

	"github.com/Tangerg/lynx/app2/runtime/domain/codebase"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const embeddingBatchSize = 32

func embeddingRoleID(role protocol.EmbeddingRole) string {
	return role.Provider + "/" + role.Model
}

func embedDocuments(
	ctx context.Context,
	model embedding.Model,
	documents []codebase.Document,
) error {
	dimensions := 0
	for start := 0; start < len(documents); start += embeddingBatchSize {
		end := min(start+embeddingBatchSize, len(documents))
		texts := make([]string, end-start)
		for index := start; index < end; index++ {
			texts[index-start] = documents[index].Snippet
		}
		vectors, err := embedTexts(ctx, model, texts)
		if err != nil {
			return err
		}
		if dimensions == 0 {
			dimensions = len(vectors[0])
		}
		for index, vector := range vectors {
			if len(vector) != dimensions {
				return fmt.Errorf(
					"codebaseflow: embedding dimensions changed from %d to %d",
					dimensions,
					len(vector),
				)
			}
			documents[start+index].Vector = vector
		}
	}
	return nil
}

func embedTexts(
	ctx context.Context,
	model embedding.Model,
	texts []string,
) ([][]float64, error) {
	request, err := embedding.NewRequest(texts)
	if err != nil {
		return nil, fmt.Errorf("codebaseflow: build embedding request: %w", err)
	}
	response, err := model.Call(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("codebaseflow: call embedding model: %w", err)
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("codebaseflow: validate embedding response: %w", err)
	}
	if len(response.Results) != len(texts) {
		return nil, fmt.Errorf(
			"codebaseflow: embedding response returned %d results for %d texts",
			len(response.Results),
			len(texts),
		)
	}
	vectors := make([][]float64, len(response.Results))
	for index, result := range response.Results {
		vectors[index] = slices.Clone(result.Embedding)
	}
	return vectors, nil
}

func rankDocuments(
	query []float64,
	documents []codebase.Document,
) []protocol.CodebaseHit {
	hits := make([]protocol.CodebaseHit, 0, len(documents))
	for _, document := range documents {
		score := cosine(query, document.Vector)
		if score <= 0 {
			continue
		}
		hits = append(hits, protocol.CodebaseHit{
			Path:      document.Path,
			StartLine: document.StartLine,
			EndLine:   document.EndLine,
			Snippet:   document.Snippet,
			Score:     score,
		})
	}
	slices.SortFunc(hits, func(left, right protocol.CodebaseHit) int {
		if left.Score > right.Score {
			return -1
		}
		if left.Score < right.Score {
			return 1
		}
		if order := strings.Compare(left.Path, right.Path); order != 0 {
			return order
		}
		if left.StartLine != right.StartLine {
			return left.StartLine - right.StartLine
		}
		return left.EndLine - right.EndLine
	})
	return hits
}

func cosine(left, right []float64) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for index := range left {
		if !finite(left[index]) || !finite(right[index]) {
			return 0
		}
		dot += left[index] * right[index]
		leftNorm += left[index] * left[index]
		rightNorm += right[index] * right[index]
	}
	if leftNorm == 0 || rightNorm == 0 ||
		!finite(dot) || !finite(leftNorm) || !finite(rightNorm) {
		return 0
	}
	score := dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
	if !finite(score) {
		return 0
	}
	return max(0, min(1, score))
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
