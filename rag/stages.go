package rag

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/Tangerg/lynx/core/document"
)

var (
	// ErrInvalidCandidate reports a missing/invalid document or non-finite score.
	ErrInvalidCandidate = errors.New("rag: invalid retrieval candidate")
	// ErrNilTransformer reports a missing query transformation capability.
	ErrNilTransformer = errors.New("rag: transformer must not be nil")
	// ErrNilExpander reports a missing query expansion capability.
	ErrNilExpander = errors.New("rag: expander must not be nil")
	// ErrNilRefiner reports a missing candidate refinement capability.
	ErrNilRefiner = errors.New("rag: refiner must not be nil")
	// ErrEmptyExpansion reports an expander that returned no queries.
	ErrEmptyExpansion = errors.New("rag: expander returned no queries")
	// ErrInvalidExpansion reports a concrete expander that could not satisfy
	// its declared expansion contract.
	ErrInvalidExpansion = errors.New("rag: invalid query expansion")
)

// Candidate relates a document to the retrieval operation that produced it.
// Score is query-specific and therefore does not belong on document.Document.
type Candidate struct {
	Document *document.Document `json:"document"`
	Score    float64            `json:"score"`
}

// Candidates is an ordered retrieval result. Its methods never mutate the
// receiver, preserving declaration and retrieval order where scores tie.
type Candidates []Candidate

// Validate checks every candidate in order.
func (c Candidates) Validate() error {
	for index, candidate := range c {
		if err := candidate.Validate(); err != nil {
			return fmt.Errorf("rag: candidate %d: %w", index, err)
		}
	}
	return nil
}

// uniqueBest returns the highest-scoring candidate for each known document
// identity. Identity-free documents remain distinct.
func (c Candidates) uniqueBest() Candidates {
	positions := make(map[string]int, len(c))
	unique := make(Candidates, 0, len(c))
	for _, candidate := range c {
		id := candidate.Document.ID
		if id == "" {
			unique = append(unique, candidate)
			continue
		}
		position, exists := positions[id]
		if !exists {
			positions[id] = len(unique)
			unique = append(unique, candidate)
			continue
		}
		if candidate.Score > unique[position].Score {
			unique[position] = candidate
		}
	}
	return unique
}

// ranked returns an independent score-descending snapshot. Stable sorting
// retains retrieval order when scores tie.
func (c Candidates) ranked() Candidates {
	ranked := slices.Clone(c)
	slices.SortStableFunc(ranked, func(left, right Candidate) int {
		return cmp.Compare(right.Score, left.Score)
	})
	return ranked
}

// Validate checks the candidate's document and score.
func (c Candidate) Validate() error {
	if c.Document == nil {
		return fmt.Errorf("%w: document must not be nil", ErrInvalidCandidate)
	}
	if err := c.Document.Validate(); err != nil {
		return fmt.Errorf("%w: document: %w", ErrInvalidCandidate, err)
	}
	if math.IsNaN(c.Score) || math.IsInf(c.Score, 0) {
		return fmt.Errorf("%w: score must be finite", ErrInvalidCandidate)
	}
	return nil
}

// Transformer rewrites a query to be more retrieval-friendly — translation,
// compression, ambiguity resolution, vocabulary normalization.
type Transformer interface {
	// Transform returns the rewritten query.
	Transform(ctx context.Context, query Query) (Query, error)
}

// TransformerFunc adapts a function to [Transformer].
type TransformerFunc func(context.Context, Query) (Query, error)

// Transform calls t(ctx, query).
func (t TransformerFunc) Transform(ctx context.Context, query Query) (Query, error) {
	return t(ctx, query)
}

// Expander turns one query into many — useful for poorly formed inputs
// (alternative phrasings) or complex problems (decompose into sub-queries).
type Expander interface {
	// Expand returns one or more queries derived from the input.
	Expand(ctx context.Context, query Query) ([]Query, error)
}

// ExpanderFunc adapts a function to [Expander].
type ExpanderFunc func(context.Context, Query) ([]Query, error)

// Expand calls e(ctx, query).
func (e ExpanderFunc) Expand(ctx context.Context, query Query) ([]Query, error) {
	return e(ctx, query)
}

// Retriever pulls candidate documents from a knowledge source.
type Retriever interface {
	// Retrieve returns documents relevant to the query.
	Retrieve(ctx context.Context, query Query) (Candidates, error)
}

// RetrieverFunc adapts a function to [Retriever].
type RetrieverFunc func(context.Context, Query) (Candidates, error)

// Retrieve calls r(ctx, query).
func (r RetrieverFunc) Retrieve(ctx context.Context, query Query) (Candidates, error) {
	return r(ctx, query)
}

// Refiner narrows candidate documents down to what the LLM should see.
type Refiner interface {
	// Refine returns the trimmed/re-ranked document list.
	Refine(ctx context.Context, query Query, candidates Candidates) (Candidates, error)
}

// RefinerFunc adapts a function to [Refiner].
type RefinerFunc func(context.Context, Query, Candidates) (Candidates, error)

// Refine calls r(ctx, query, candidates).
func (r RefinerFunc) Refine(ctx context.Context, query Query, candidates Candidates) (Candidates, error) {
	return r(ctx, query, candidates)
}
